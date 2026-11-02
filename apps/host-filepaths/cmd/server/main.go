package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	mediaRoot      string
	qbittorrentURL string
	qbittorrentUser string
}

type BrowseEntry struct {
	Name string `json:"name"`
}

type BrowseResult struct {
	Path   string        `json:"path"`
	Parent string        `json:"parent"`
	Dirs   []BrowseEntry `json:"dirs"`
	Files  []BrowseEntry `json:"files"`
}

type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (s *Server) lanOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ip := net.ParseIP(host)
		if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next(sw, r)
		dur := time.Since(start).Round(time.Millisecond)

		isGet := r.Method == http.MethodGet
		isError := sw.status >= 400
		if !isGet || isError {
			q := r.URL.RawQuery
			if q != "" {
				q = "?" + q
			}
			result := "OK"
			if isError {
				result = "FAIL"
			}
			log.Printf("[%s] %s %s%s %d %s (%s)", result, r.Method, r.URL.Path, q, sw.status, host, dur)
		}
	}
}

// resolveBrowsePath validates relPath against the media root, preventing
// traversal outside it via ".." segments or symlinks.
func (s *Server) resolveBrowsePath(relPath string) (string, error) {
	cleaned := filepath.Clean("/" + relPath)
	joined := filepath.Join(s.mediaRoot, cleaned)
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(s.mediaRoot)
	if err != nil {
		return "", err
	}
	if resolved != rootResolved && !strings.HasPrefix(resolved, rootResolved+string(filepath.Separator)) {
		return "", os.ErrPermission
	}
	return resolved, nil
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	dir, err := s.resolveBrowsePath(relPath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	result := BrowseResult{Path: filepath.Clean("/" + relPath), Dirs: []BrowseEntry{}, Files: []BrowseEntry{}}
	if result.Path != "/" {
		result.Parent = filepath.Dir(result.Path)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			result.Dirs = append(result.Dirs, BrowseEntry{Name: e.Name()})
		} else {
			result.Files = append(result.Files, BrowseEntry{Name: e.Name()})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// qbittorrentAuth logs into qBittorrent's WebAPI and returns the SID cookie.
// qBittorrent requires the Referer/Origin headers to match the request Host,
// or the login is rejected even with correct credentials.
func (s *Server) qbittorrentAuth(password string) (*http.Cookie, error) {
	form := url.Values{"username": {s.qbittorrentUser}, "password": {password}}
	req, err := http.NewRequest(http.MethodPost, s.qbittorrentURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", s.qbittorrentURL)
	req.Header.Set("Origin", s.qbittorrentURL)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// qBittorrent 4.x names the session cookie "SID"; 5.x names it
	// "QBT_SID_<port>" instead. Accept either rather than hardcoding one.
	for _, c := range resp.Cookies() {
		if c.Name == "SID" || strings.HasPrefix(c.Name, "QBT_SID") {
			return c, nil
		}
	}
	return nil, os.ErrPermission
}

func (s *Server) qbittorrentLogout(sid *http.Cookie) {
	req, err := http.NewRequest(http.MethodPost, s.qbittorrentURL+"/api/v2/auth/logout", nil)
	if err != nil {
		return
	}
	req.AddCookie(sid)
	req.Header.Set("Referer", s.qbittorrentURL)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func (s *Server) qbittorrentListCategories(sid *http.Cookie) (map[string]Category, error) {
	req, err := http.NewRequest(http.MethodGet, s.qbittorrentURL+"/api/v2/torrents/categories", nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(sid)
	req.Header.Set("Referer", s.qbittorrentURL)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cats map[string]Category
	if err := json.NewDecoder(resp.Body).Decode(&cats); err != nil {
		return nil, err
	}
	return cats, nil
}

func (s *Server) qbittorrentSetCategoryPath(sid *http.Cookie, category, savePath string) error {
	cats, err := s.qbittorrentListCategories(sid)
	if err != nil {
		return err
	}

	endpoint := "/api/v2/torrents/createCategory"
	if _, exists := cats[category]; exists {
		endpoint = "/api/v2/torrents/editCategory"
	}

	form := url.Values{"category": {category}, "savePath": {savePath}}
	req, err := http.NewRequest(http.MethodPost, s.qbittorrentURL+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", s.qbittorrentURL)
	req.AddCookie(sid)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return os.ErrInvalid
	}
	return nil
}

func (s *Server) qbittorrentRemoveCategory(sid *http.Cookie, category string) error {
	form := url.Values{"categories": {category}}
	req, err := http.NewRequest(http.MethodPost, s.qbittorrentURL+"/api/v2/torrents/removeCategories", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", s.qbittorrentURL)
	req.AddCookie(sid)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return os.ErrInvalid
	}
	return nil
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	password := os.Getenv("QBITTORRENT_PASSWORD")
	if password == "" {
		http.Error(w, `{"error":"QBITTORRENT_PASSWORD not configured"}`, http.StatusBadGateway)
		return
	}

	sid, err := s.qbittorrentAuth(password)
	if err != nil {
		http.Error(w, `{"error":"qbittorrent auth failed"}`, http.StatusBadGateway)
		return
	}
	defer s.qbittorrentLogout(sid)

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		cats, err := s.qbittorrentListCategories(sid)
		if err != nil {
			http.Error(w, `{"error":"failed to list categories"}`, http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(cats)

	case http.MethodPost:
		var req struct {
			Category string `json:"category"`
			Path     string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Category == "" || req.Path == "" {
			http.Error(w, `{"error":"category and path are required"}`, http.StatusBadRequest)
			return
		}
		if _, err := s.resolveBrowsePath(strings.TrimPrefix(req.Path, s.mediaRoot)); err != nil {
			http.Error(w, `{"error":"path must be under the media root"}`, http.StatusBadRequest)
			return
		}
		if err := s.qbittorrentSetCategoryPath(sid, req.Category, req.Path); err != nil {
			http.Error(w, `{"error":"failed to set category path"}`, http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case http.MethodDelete:
		var req struct {
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Category == "" {
			http.Error(w, `{"error":"category is required"}`, http.StatusBadRequest)
			return
		}
		if err := s.qbittorrentRemoveCategory(sid, req.Category); err != nil {
			http.Error(w, `{"error":"failed to remove category"}`, http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	addr := flag.String("addr", "0.0.0.0:5011", "listen address")
	webDir := flag.String("web", "web", "static web directory")
	media := flag.String("media", "/mnt/server-pc/Media", "media root directory to browse")
	qbURL := flag.String("qbittorrent-url", "http://192.168.15.1:8090", "qBittorrent WebUI base URL")
	qbUser := flag.String("qbittorrent-user", "admin", "qBittorrent WebUI username")
	flag.Parse()

	s := &Server{
		mediaRoot:       *media,
		qbittorrentURL:  strings.TrimRight(*qbURL, "/"),
		qbittorrentUser: *qbUser,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/browse", s.lanOnly(s.handleBrowse))
	mux.HandleFunc("/api/categories", s.lanOnly(s.handleCategories))
	mux.Handle("/", http.FileServer(http.Dir(*webDir)))

	log.Printf("host-filepaths listening on %s (media root: %s, qbittorrent: %s)", *addr, *media, s.qbittorrentURL)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
