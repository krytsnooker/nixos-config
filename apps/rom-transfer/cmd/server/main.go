package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Console struct {
	Name       string `json:"name"`
	ServerPath string `json:"server_path"`
}

type HostConfig struct {
	Consoles  []Console `json:"consoles"`
	SavesPath string    `json:"saves_path"`
}

type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type Server struct {
	mu         sync.RWMutex
	cfg        HostConfig
	configPath string
	webDir     string
}

func main() {
	addr := flag.String("addr", ":5035", "listen address")
	configPath := flag.String("config", "hostconfig.json", "host config path")
	webDir := flag.String("web", "web", "web files directory")
	flag.Parse()

	s := &Server{
		configPath: *configPath,
		webDir:     *webDir,
	}
	if data, err := os.ReadFile(*configPath); err == nil {
		json.Unmarshal(data, &s.cfg)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/consoles", s.lanOnly(s.handleConsoles))
	mux.HandleFunc("/api/files", s.lanOnly(s.handleFiles))
	mux.HandleFunc("/api/download", s.lanOnly(s.handleDownload))
	mux.HandleFunc("/api/hostconfig", s.lanOnly(s.handleHostConfig))
	mux.HandleFunc("/api/saves", s.lanOnly(s.handleSaves))
	mux.HandleFunc("/api/saves/download", s.lanOnly(s.handleSavesDownload))
	mux.HandleFunc("/api/saves/upload", s.lanOnly(s.handleSavesUpload))
	mux.Handle("/", http.FileServer(http.Dir(*webDir)))

	log.Printf("rom-transfer server on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
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
		next(w, r)
	}
}

func (s *Server) handleConsoles(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	out := s.cfg.Consoles
	if out == nil {
		out = []Console{}
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleHostConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.cfg)
	case http.MethodPut:
		var cfg HostConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(s.configPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.mu.Lock()
		s.cfg = cfg
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) consolePath(name string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cfg.Consoles {
		if c.Name == name {
			return c.ServerPath, true
		}
	}
	return "", false
}

func listDir(dir string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{Name: e.Name(), Size: info.Size()})
	}
	return files, nil
}

// safePath validates a bare filename and returns its full path inside dir.
// Rejects path separators, "..", and symlinks escaping dir.
func safePath(dir, filename string) (string, bool) {
	if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") || filename == "" {
		return "", false
	}
	p := filepath.Join(dir, filename)
	rel, err := filepath.Rel(dir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	info, err := os.Lstat(p)
	if err != nil {
		return "", false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			return "", false
		}
		if rel2, err := filepath.Rel(dir, target); err != nil || strings.HasPrefix(rel2, "..") {
			return "", false
		}
	}
	return p, true
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	dir, ok := s.consolePath(console)
	if !ok {
		http.Error(w, "unknown console", http.StatusBadRequest)
		return
	}
	files, err := listDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []FileInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	file := r.URL.Query().Get("file")
	dir, ok := s.consolePath(console)
	if !ok {
		http.Error(w, "unknown console", http.StatusBadRequest)
		return
	}
	p, ok := safePath(dir, file)
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, p)
}

func (s *Server) savesDir(console string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.SavesPath == "" || console == "" {
		return "", false
	}
	return filepath.Join(s.cfg.SavesPath, console), true
}

func (s *Server) handleSaves(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	dir, ok := s.savesDir(console)
	if !ok {
		http.Error(w, "saves path not configured", http.StatusBadRequest)
		return
	}
	os.MkdirAll(dir, 0755)
	files, err := listDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []FileInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleSavesDownload(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	file := r.URL.Query().Get("file")
	dir, ok := s.savesDir(console)
	if !ok {
		http.Error(w, "saves path not configured", http.StatusBadRequest)
		return
	}
	p, ok := safePath(dir, file)
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, p)
}

func (s *Server) handleSavesUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	console := r.URL.Query().Get("console")
	file := r.URL.Query().Get("file")
	dir, ok := s.savesDir(console)
	if !ok {
		http.Error(w, "saves path not configured", http.StatusBadRequest)
		return
	}
	if strings.ContainsAny(file, "/\\") || strings.Contains(file, "..") || file == "" {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	os.MkdirAll(dir, 0755)
	dest := filepath.Join(dir, file)
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(f, r.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
