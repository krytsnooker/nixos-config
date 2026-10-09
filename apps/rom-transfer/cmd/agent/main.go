package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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
	"time"
)

const version = "1.10"

const maxLogEntries = 500

type logEntry struct {
	T   string `json:"t"`
	Lvl string `json:"lvl"`
	Msg string `json:"msg"`
}

type logBuffer struct {
	mu      sync.Mutex
	entries []logEntry
}

func (lb *logBuffer) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}
	lvl := "info"
	upper := strings.ToUpper(line)
	if strings.Contains(upper, "FAILED") || strings.Contains(upper, "ERROR") ||
		strings.Contains(upper, "FATAL") {
		lvl = "fail"
	} else if strings.Contains(upper, "COMPLETE") || strings.Contains(upper, "INSTALLED") ||
		strings.Contains(upper, "SHORTCUT ADDED") {
		lvl = "ok"
	}
	e := logEntry{T: time.Now().Format("2006-01-02 15:04:05"), Lvl: lvl, Msg: line}
	lb.mu.Lock()
	lb.entries = append(lb.entries, e)
	if len(lb.entries) > maxLogEntries {
		lb.entries = lb.entries[len(lb.entries)-maxLogEntries:]
	}
	lb.mu.Unlock()
	return len(p), nil
}

var reTimestamp = regexp.MustCompile(`_\d{8}-\d{6}$`)

func stripTimestamp(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return reTimestamp.ReplaceAllString(base, "") + ext
}

type Config struct {
	ServerURL     string            `json:"server_url"`
	Roms          map[string]string `json:"roms"`
	Saves         map[string]string `json:"saves"`
	Games         map[string]string `json:"games"`
	SteamPath     string            `json:"steam_path"`
	SteamProfiles map[string]string `json:"steam_profiles"`
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
	Hash string `json:"hash,omitempty"`
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

type DiskInfo struct {
	FolderSize int64 `json:"folder_size"`
	FreeSpace  int64 `json:"free_space"`
}

type TransferJob struct {
	Kind     string `json:"kind"`      // "" or "rom" = ROM; "pc" = PC game
	Console  string `json:"console"`
	File     string `json:"file"`      // ROM job
	Item     string `json:"item"`      // PC job: item name on server
	ItemType string `json:"item_type"` // PC job: "folder" or "archive"
}

const extractingTotal = int64(-1)

// ActiveTransfer uses an atomic counter for bytes so the status handler
// can read progress without holding the queue lock during IO.
type ActiveTransfer struct {
	Kind    string `json:"kind"`
	Console string `json:"console"`
	File    string `json:"file"`
	Item    string `json:"item,omitempty"`
	Total   int64  `json:"total"`
	abytes  atomic.Int64
}

func (a *ActiveTransfer) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind    string  `json:"kind"`
		Console string  `json:"console"`
		File    string  `json:"file"`
		Item    string  `json:"item,omitempty"`
		Stage   string  `json:"stage,omitempty"`
		Bytes   int64   `json:"bytes"`
		Total   int64   `json:"total"`
		Percent float64 `json:"percent"`
	}
	b := a.abytes.Load()
	var pct float64
	stage := ""
	total := a.Total
	if total == extractingTotal {
		stage = "extracting"
		total = 0
	} else if total > 0 {
		pct = float64(b) / float64(total) * 100
	}
	return json.Marshal(wire{Kind: a.Kind, Console: a.Console, File: a.File, Item: a.Item, Stage: stage, Bytes: b, Total: total, Percent: pct})
}

type Agent struct {
	mu         sync.RWMutex
	cfg        Config
	configPath string

	qmu    sync.Mutex
	queue  []TransferJob
	active *ActiveTransfer
	jobCh  chan struct{}

	logs *logBuffer
}

func newAgent(configPath string) *Agent {
	lb := &logBuffer{}
	log.SetOutput(io.MultiWriter(os.Stderr, lb))
	log.SetFlags(0)
	a := &Agent{
		configPath: configPath,
		jobCh:      make(chan struct{}, 1),
		logs:       lb,
	}
	a.loadConfig()
	return a
}

func (a *Agent) loadConfig() {
	log.Printf("config path: %s", a.configPath)
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		log.Printf("config not found (%v), using defaults", err)
		a.cfg = Config{
			Roms:          map[string]string{},
			Saves:         map[string]string{},
			Games:         map[string]string{},
			SteamProfiles: map[string]string{},
		}
		return
	}
	log.Printf("config loaded (%d bytes)", len(data))
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
	if cfg.Games == nil {
		cfg.Games = map[string]string{}
	}
	if cfg.SteamProfiles == nil {
		cfg.SteamProfiles = map[string]string{}
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
	fmt.Fprintf(w, `{"status":"ok","version":%q}`, version)
}

func (a *Agent) handleLogs(w http.ResponseWriter, r *http.Request) {
	a.logs.mu.Lock()
	entries := make([]logEntry, len(a.logs.entries))
	copy(entries, a.logs.entries)
	a.logs.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
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
		if cfg.Games == nil {
			cfg.Games = map[string]string{}
		}
		if cfg.SteamProfiles == nil {
			cfg.SteamProfiles = map[string]string{}
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

// ── PC Games ──────────────────────────────────────────────────────────────────

type GameEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"` // -1 = unknown
}

func (a *Agent) handleGames(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	if console == "" {
		http.Error(w, "console required", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	dir           := a.cfg.Games[console]
	steamPath     := a.cfg.SteamPath
	steamProfiles := a.cfg.SteamProfiles
	a.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		if dir == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"games":     []GameEntry{},
				"disk_info": nil,
			})
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var games []GameEntry
		for _, e := range entries {
			if e.IsDir() {
				games = append(games, GameEntry{Name: e.Name(), Size: -1})
			}
		}
		if games == nil {
			games = []GameEntry{}
		}
		free, _ := freeSpace(dir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"games":     games,
			"disk_info": DiskInfo{FreeSpace: int64(free)},
		})

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		if dir == "" {
			http.Error(w, "no games directory configured", http.StatusBadRequest)
			return
		}
		gamePath := filepath.Join(dir, name)
		rel, err := filepath.Rel(dir, gamePath)
		if err != nil || strings.HasPrefix(rel, "..") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		// Remove Steam shortcut before deleting folder
		if steamPath != "" {
			if userID := steamProfiles[console]; userID != "" {
				if err := RemoveSteamShortcut(steamPath, userID, name); err != nil {
					log.Printf("remove steam shortcut %q: %v", name, err)
				}
			}
		}
		if err := os.RemoveAll(gamePath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *Agent) handleGamesExes(w http.ResponseWriter, r *http.Request) {
	console := r.URL.Query().Get("console")
	item    := r.URL.Query().Get("item")
	a.mu.RLock()
	gamesDir := a.cfg.Games[console]
	a.mu.RUnlock()
	if gamesDir == "" {
		http.Error(w, "no games directory configured for this console", http.StatusBadRequest)
		return
	}
	gameName := strings.TrimSuffix(item, filepath.Ext(item))
	gamePath := filepath.Join(gamesDir, gameName)

	type ExeEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	var exes []ExeEntry
	filepath.Walk(gamePath, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(p)) == ".exe" {
			exes = append(exes, ExeEntry{Name: fi.Name(), Path: p})
		}
		return nil
	})
	if exes == nil {
		exes = []ExeEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exes)
}

func (a *Agent) handleSteamAccounts(w http.ResponseWriter, r *http.Request) {
	steamPath := r.URL.Query().Get("path")
	if steamPath == "" {
		a.mu.RLock()
		steamPath = a.cfg.SteamPath
		a.mu.RUnlock()
	}
	if steamPath == "" {
		http.Error(w, "steam path not configured", http.StatusBadRequest)
		return
	}
	accounts, err := FindSteamAccounts(steamPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

func (a *Agent) handleSteamShortcut(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	steamPath     := a.cfg.SteamPath
	steamProfiles := a.cfg.SteamProfiles
	a.mu.RUnlock()

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Console  string `json:"console"`
			Name     string `json:"name"`
			Exe      string `json:"exe"`
			StartDir string `json:"start_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if steamPath == "" {
			http.Error(w, "steam path not configured", http.StatusBadRequest)
			return
		}
		userID := steamProfiles[req.Console]
		if userID == "" {
			http.Error(w, "no steam account mapped for console "+req.Console, http.StatusBadRequest)
			return
		}
		startDir := req.StartDir
		if startDir == "" {
			startDir = filepath.Dir(req.Exe)
		}
		if err := AddSteamShortcut(steamPath, userID, req.Name, req.Exe, startDir); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		console := r.URL.Query().Get("console")
		name    := r.URL.Query().Get("name")
		if steamPath == "" {
			http.Error(w, "steam path not configured", http.StatusBadRequest)
			return
		}
		userID := steamProfiles[console]
		if userID == "" {
			http.Error(w, "no steam account mapped for console "+console, http.StatusBadRequest)
			return
		}
		if err := RemoveSteamShortcut(steamPath, userID, name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── Transfer ──────────────────────────────────────────────────────────────────

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
	if job.Console == "" {
		http.Error(w, "console required", http.StatusBadRequest)
		return
	}
	if job.Kind == "pc" {
		if job.Item == "" {
			http.Error(w, "item required for pc job", http.StatusBadRequest)
			return
		}
	} else {
		if !safeFilename(job.File) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
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

// ── Saves ─────────────────────────────────────────────────────────────────────

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
	for i := range files {
		files[i].Hash = fileHash(filepath.Join(folder, files[i].Name))
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

// ── Transfer worker ───────────────────────────────────────────────────────────

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
	at := &ActiveTransfer{Kind: job.Kind, Console: job.Console, File: job.File, Item: job.Item}
	a.active = at
	a.qmu.Unlock()

	label := job.Console + "/" + job.File + job.Item
	log.Printf("transfer started: %s", label)
	var err error
	if job.Kind == "pc" {
		err = a.doPCTransfer(job, at)
	} else {
		err = a.doTransfer(at)
	}
	if err != nil {
		log.Printf("transfer FAILED: %s — %v", label, err)
	} else {
		log.Printf("transfer complete: %s", label)
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

	if at.Total > 0 {
		if info, err := os.Stat(dest); err == nil && info.Size() == at.Total {
			at.abytes.Store(at.Total)
			return nil
		}
	}

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

func (a *Agent) doPCTransfer(job TransferJob, at *ActiveTransfer) error {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()

	destDir := cfg.Games[job.Console]
	if destDir == "" {
		return fmt.Errorf("no games destination configured for %s", job.Console)
	}

	fileURL := cfg.ServerURL + "/api/download?console=" + url.QueryEscape(job.Console) + "&item=" + url.QueryEscape(job.Item)

	// HEAD is best-effort — folder zips have no Content-Length
	if headResp, err := http.Head(fileURL); err == nil {
		headResp.Body.Close()
		if headResp.ContentLength > 0 {
			at.Total = headResp.ContentLength
		}
	}

	resp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	if at.Total == 0 && resp.ContentLength > 0 {
		at.Total = resp.ContentLength
	}

	tmp, err := os.CreateTemp("", "rom-pc-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, &progressReader{r: resp.Body, at: at}); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	// Signal extracting phase to the status poller
	at.Total = extractingTotal
	at.abytes.Store(0)

	gameName := strings.TrimSuffix(job.Item, filepath.Ext(job.Item))
	destPath := filepath.Join(destDir, gameName)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(job.Item))
	if job.ItemType == "folder" || ext == ".zip" {
		return extractZip(tmpPath, destPath)
	}
	return extract7z(tmpPath, destPath)
}

func extractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		rel, err := filepath.Rel(destDir, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
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

// ── Scan / Update ─────────────────────────────────────────────────────────────

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

func (a *Agent) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.mu.RLock()
	serverURL := strings.TrimRight(a.cfg.ServerURL, "/")
	a.mu.RUnlock()
	if serverURL == "" {
		http.Error(w, "server URL not configured", http.StatusBadRequest)
		return
	}
	if err := doUpdate(serverURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	addr   := flag.String("addr", "127.0.0.1:5031", "listen address")
	uninst := flag.Bool("uninstall", false, "uninstall the system service (Windows, run as administrator)")
	flag.Parse()

	if *uninst {
		if err := uninstallService(); err != nil {
			log.Fatalf("uninstall failed: %v", err)
		}
		log.Println("Service uninstalled.")
		return
	}

	a := newAgent(configFilePath())
	go a.runWorker()

	mux := http.NewServeMux()
	mux.HandleFunc("/health",          a.cors(a.handleHealth))
	mux.HandleFunc("/config",          a.cors(a.handleConfig))
	mux.HandleFunc("/files",           a.cors(a.handleFiles))
	mux.HandleFunc("/transfer",        a.cors(a.handleTransfer))
	mux.HandleFunc("/transfer/status", a.cors(a.handleTransferStatus))
	mux.HandleFunc("/browse",          a.cors(a.handleBrowse))
	mux.HandleFunc("/saves",           a.cors(a.handleSaves))
	mux.HandleFunc("/saves/push",      a.cors(a.handleSavesPush))
	mux.HandleFunc("/saves/pull",      a.cors(a.handleSavesPull))
	mux.HandleFunc("/scan",            a.cors(a.handleScan))
	mux.HandleFunc("/update",          a.cors(a.handleUpdate))
	mux.HandleFunc("/games",           a.cors(a.handleGames))
	mux.HandleFunc("/games/exes",      a.cors(a.handleGamesExes))
	mux.HandleFunc("/steam/accounts",  a.cors(a.handleSteamAccounts))
	mux.HandleFunc("/steam/shortcut",  a.cors(a.handleSteamShortcut))
	mux.HandleFunc("/logs",            a.cors(a.handleLogs))

	srv := &http.Server{Addr: *addr, Handler: mux}
	startServer(srv, *addr)
}
