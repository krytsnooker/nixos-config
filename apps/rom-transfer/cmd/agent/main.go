package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

const version = "1.4"

var reTimestamp = regexp.MustCompile(`_\d{8}-\d{6}$`)

func stripTimestamp(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return reTimestamp.ReplaceAllString(base, "") + ext
}

type Config struct {
	ServerURL string            `json:"server_url"`
	Roms      map[string]string `json:"roms"`
	Saves     map[string]string `json:"saves"`
}

type ScanResult struct {
	Emulator  string `json:"emulator"`
	Path      string `json:"path"`
	Console   string `json:"console"`
	FileCount int    `json:"file_count"`
}

type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type DiskInfo struct {
	FolderSize int64 `json:"folder_size"`
	FreeSpace  int64 `json:"free_space"`
}

type TransferJob struct {
	Console string `json:"console"`
	File    string `json:"file"`
}

// ActiveTransfer uses an atomic counter for bytes so the status handler
// can read progress without holding the queue lock during IO.
type ActiveTransfer struct {
	Console string `json:"console"`
	File    string `json:"file"`
	Total   int64  `json:"total"`
	abytes  atomic.Int64
}

func (a *ActiveTransfer) MarshalJSON() ([]byte, error) {
	type wire struct {
		Console string  `json:"console"`
		File    string  `json:"file"`
		Bytes   int64   `json:"bytes"`
		Total   int64   `json:"total"`
		Percent float64 `json:"percent"`
	}
	b := a.abytes.Load()
	var pct float64
	if a.Total > 0 {
		pct = float64(b) / float64(a.Total) * 100
	}
	return json.Marshal(wire{Console: a.Console, File: a.File, Bytes: b, Total: a.Total, Percent: pct})
}

type Agent struct {
	mu         sync.RWMutex
	cfg        Config
	configPath string

	qmu    sync.Mutex
	queue  []TransferJob
	active *ActiveTransfer
	jobCh  chan struct{}
}

func newAgent(configPath string) *Agent {
	a := &Agent{
		configPath: configPath,
		jobCh:      make(chan struct{}, 1),
	}
	a.loadConfig()
	return a
}

func (a *Agent) loadConfig() {
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		a.cfg = Config{Roms: map[string]string{}, Saves: map[string]string{}}
		return
	}
	// migration: old config had a flat "destinations" field
	var raw map[string]json.RawMessage
	var cfg Config
	if err := json.Unmarshal(data, &raw); err == nil {
		if dest, ok := raw["destinations"]; ok && raw["roms"] == nil {
			var destinations map[string]string
			if json.Unmarshal(dest, &destinations) == nil {
				cfg.Roms = destinations
			}
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	if cfg.Roms == nil {
		cfg.Roms = map[string]string{}
	}
	if cfg.Saves == nil {
		cfg.Saves = map[string]string{}
	}
	a.cfg = cfg
}

func (a *Agent) writeConfig(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(a.configPath), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.configPath, data, 0600)
}

// cors wraps a handler with CORS and Private Network Access headers.
// Loopback origins (localhost / 127.0.0.1) are always allowed so the app
// works when accessed via either hostname while the server_url is set to
// the LAN IP. Non-loopback origins must match the configured server_url.
func (a *Agent) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func (a *Agent) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (a *Agent) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.mu.RLock()
		defer a.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(a.cfg)
	case http.MethodPut:
		var cfg Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if cfg.Roms == nil {
			cfg.Roms = map[string]string{}
		}
		if cfg.Saves == nil {
			cfg.Saves = map[string]string{}
		}
		if err := a.writeConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.mu.Lock()
		a.cfg = cfg
		a.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func safeFilename(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/\\") && !strings.Contains(s, "..")
}

func listDir(dir string) ([]FileInfo, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	var files []FileInfo
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{Name: e.Name(), Size: info.Size()})
		total += info.Size()
	}
	return files, total, nil
}

func (a *Agent) handleFiles(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	if console == "" {
		http.Error(w, "console required", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	dir := a.cfg.Roms[console]
	a.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		if dir == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"files":     []FileInfo{},
				"disk_info": nil,
			})
			return
		}
		files, folderSize, _ := listDir(dir)
		if files == nil {
			files = []FileInfo{}
		}
		free, _ := freeSpace(dir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"files": files,
			"disk_info": DiskInfo{
				FolderSize: folderSize,
				FreeSpace:  int64(free),
			},
		})

	case http.MethodDelete:
		if dir == "" {
			http.Error(w, "no destination configured for this console", http.StatusBadRequest)
			return
		}
		file := r.URL.Query().Get("file")
		if !safeFilename(file) {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}
		p := filepath.Join(dir, file)
		rel, err := filepath.Rel(dir, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		if err := os.Remove(p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *Agent) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var job TransferJob
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if job.Console == "" || !safeFilename(job.File) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	a.qmu.Lock()
	a.queue = append(a.queue, job)
	a.qmu.Unlock()
	select {
	case a.jobCh <- struct{}{}:
	default:
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *Agent) handleTransferStatus(w http.ResponseWriter, r *http.Request) {
	a.qmu.Lock()
	active := a.active
	queue := make([]TransferJob, len(a.queue))
	copy(queue, a.queue)
	a.qmu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active": active,
		"queue":  queue,
	})
}

func (a *Agent) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = defaultBrowseRoot()
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(path, e.Name()))
		}
	}
	if dirs == nil {
		dirs = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":   path,
		"parent": filepath.Dir(path),
		"dirs":   dirs,
	})
}

func (a *Agent) handleSaves(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	a.mu.RLock()
	folder := a.cfg.Saves[console]
	a.mu.RUnlock()
	if folder == "" {
		folder = defaultDownloadsDir()
	}
	os.MkdirAll(folder, 0755)
	files, _, _ := listDir(folder)
	if files == nil {
		files = []FileInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (a *Agent) handleSavesPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Console string `json:"console"`
		File    string `json:"file"`
		Game    string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !safeFilename(req.File) {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	folder := cfg.Saves[req.Console]
	if folder == "" {
		folder = defaultDownloadsDir()
	}
	f, err := os.Open(filepath.Join(folder, req.File))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	uploadURL := cfg.ServerURL + "/api/saves/upload?console=" + url.QueryEscape(req.Console) + "&file=" + url.QueryEscape(req.File) + "&game=" + url.QueryEscape(req.Game)
	resp, err := http.Post(uploadURL, "application/octet-stream", f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		http.Error(w, fmt.Sprintf("server %d: %s", resp.StatusCode, body), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Agent) handleSavesPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Console string `json:"console"`
		File    string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !safeFilename(req.File) {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	folder := cfg.Saves[req.Console]
	if folder == "" {
		folder = defaultDownloadsDir()
	}
	dlURL := cfg.ServerURL + "/api/saves/download?console=" + url.QueryEscape(req.Console) + "&file=" + url.QueryEscape(req.File)
	resp, err := http.Get(dlURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("server returned %d", resp.StatusCode), http.StatusInternalServerError)
		return
	}
	os.MkdirAll(folder, 0755)
	dest := filepath.Join(folder, stripTimestamp(req.File))
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
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

// Transfer worker — runs one job at a time, re-signals itself if the queue has more.

func (a *Agent) runWorker() {
	for range a.jobCh {
		a.runNextJob()
	}
}

func (a *Agent) runNextJob() {
	a.qmu.Lock()
	if len(a.queue) == 0 {
		a.qmu.Unlock()
		return
	}
	job := a.queue[0]
	a.queue = a.queue[1:]
	at := &ActiveTransfer{Console: job.Console, File: job.File}
	a.active = at
	a.qmu.Unlock()

	if err := a.doTransfer(at); err != nil {
		log.Printf("transfer %s/%s failed: %v", job.Console, job.File, err)
	}

	a.qmu.Lock()
	a.active = nil
	hasMore := len(a.queue) > 0
	a.qmu.Unlock()

	if hasMore {
		select {
		case a.jobCh <- struct{}{}:
		default:
		}
	}
}

func (a *Agent) doTransfer(at *ActiveTransfer) error {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()

	destDir := cfg.Roms[at.Console]
	if destDir == "" {
		return fmt.Errorf("no destination configured for %s", at.Console)
	}

	fileURL := cfg.ServerURL + "/api/download?console=" + url.QueryEscape(at.Console) + "&file=" + url.QueryEscape(at.File)

	headResp, err := http.Head(fileURL)
	if err != nil {
		return fmt.Errorf("HEAD failed: %v", err)
	}
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD returned %d", headResp.StatusCode)
	}
	at.Total = headResp.ContentLength

	dest := filepath.Join(destDir, at.File)

	// Skip if we already have the complete file.
	if at.Total > 0 {
		if info, err := os.Stat(dest); err == nil && info.Size() == at.Total {
			at.abytes.Store(at.Total)
			return nil
		}
	}

	// Check free space before starting.
	if at.Total > 0 {
		free, err := freeSpace(destDir)
		if err != nil {
			return fmt.Errorf("cannot check free space: %v", err)
		}
		if uint64(at.Total) > free {
			return fmt.Errorf("not enough free space: need %s, have %s",
				fmtBytes(at.Total), fmtBytes(int64(free)))
		}
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	resp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET returned %d", resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, &progressReader{r: resp.Body, at: at})
	f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	return os.Rename(tmp, dest)
}

type progressReader struct {
	r  io.Reader
	at *ActiveTransfer
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.at.abytes.Add(int64(n))
	}
	return n, err
}

func (a *Agent) handleScan(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	serverURL := strings.TrimRight(a.cfg.ServerURL, "/")
	a.mu.RUnlock()
	if serverURL == "" {
		http.Error(w, "server URL not configured", http.StatusBadRequest)
		return
	}
	resp, err := http.Get(serverURL + "/api/scanpaths")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	var paths []struct {
		Emulator string `json:"emulator"`
		Path     string `json:"path"`
		Console  string `json:"console"`
		OS       string `json:"os"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paths); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	results := []ScanResult{}
	for _, p := range paths {
		if p.OS != "" && p.OS != agentOS {
			continue
		}
		expanded := expandPath(p.Path)
		entries, err := os.ReadDir(expanded)
		if err != nil {
			continue
		}
		count := 0
		for _, e := range entries {
			if !e.IsDir() {
				count++
			}
		}
		results = append(results, ScanResult{
			Emulator:  p.Emulator,
			Path:      expanded,
			Console:   p.Console,
			FileCount: count,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func fmtBytes(b int64) string {
	const MB = 1024 * 1024
	const GB = 1024 * MB
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:5031", "listen address")
	flag.Parse()

	a := newAgent(configFilePath())
	go a.runWorker()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.cors(a.handleHealth))
	mux.HandleFunc("/config", a.cors(a.handleConfig))
	mux.HandleFunc("/files", a.cors(a.handleFiles))
	mux.HandleFunc("/transfer", a.cors(a.handleTransfer))
	mux.HandleFunc("/transfer/status", a.cors(a.handleTransferStatus))
	mux.HandleFunc("/browse", a.cors(a.handleBrowse))
	mux.HandleFunc("/saves", a.cors(a.handleSaves))
	mux.HandleFunc("/saves/push", a.cors(a.handleSavesPush))
	mux.HandleFunc("/saves/pull", a.cors(a.handleSavesPull))
	mux.HandleFunc("/scan", a.cors(a.handleScan))

	log.Printf("rom-agent v%s listening on %s", version, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
