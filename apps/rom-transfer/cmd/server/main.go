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
	"time"
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

type SaveFileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Game string `json:"game"`
}

type ConsoleFileTypes struct {
	Rom  []string `json:"rom"`
	Save []string `json:"save"`
}

type ScanPath struct {
	Emulator string `json:"emulator"`
	Path     string `json:"path"`
	Console  string `json:"console"`
	OS       string `json:"os"`
}

var defaultScanPaths = []ScanPath{
	{Emulator: "RetroArch", Path: `%APPDATA%\RetroArch\saves`, Console: "", OS: "windows"},
	{Emulator: "PCSX2", Path: `%USERPROFILE%\Documents\PCSX2\memcards`, Console: "PS2", OS: "windows"},
	{Emulator: "DuckStation", Path: `%APPDATA%\DuckStation\memcards`, Console: "PS1", OS: "windows"},
	{Emulator: "Flycast", Path: `%APPDATA%\flycast\data`, Console: "Dreamcast", OS: "windows"},
	{Emulator: "Project64", Path: `%APPDATA%\Project64\Save`, Console: "N64", OS: "windows"},
	{Emulator: "Dolphin", Path: `%APPDATA%\Dolphin Emulator\GC`, Console: "", OS: "windows"},
	{Emulator: "Mednafen", Path: `%APPDATA%\Mednafen\sav`, Console: "", OS: "windows"},
	{Emulator: "ePSXe", Path: `%APPDATA%\ePSXe\memcards`, Console: "PS1", OS: "windows"},
	{Emulator: "Snes9x", Path: `%APPDATA%\Snes9x`, Console: "SNES", OS: "windows"},
	{Emulator: "RetroArch", Path: `~/.config/retroarch/saves`, Console: "", OS: "linux"},
	{Emulator: "PCSX2", Path: `~/.config/PCSX2/memcards`, Console: "PS2", OS: "linux"},
	{Emulator: "DuckStation", Path: `~/.local/share/duckstation/memcards`, Console: "PS1", OS: "linux"},
	{Emulator: "Flycast", Path: `~/.local/share/flycast/data`, Console: "Dreamcast", OS: "linux"},
	{Emulator: "Mupen64Plus", Path: `~/.local/share/mupen64plus/save`, Console: "N64", OS: "linux"},
	{Emulator: "Dolphin", Path: `~/.local/share/dolphin-emu/GC`, Console: "", OS: "linux"},
	{Emulator: "Mednafen", Path: `~/.mednafen/sav`, Console: "", OS: "linux"},
}

var defaultFileTypes = map[string]ConsoleFileTypes{
	"NES":       {Rom: []string{".nes"}, Save: []string{".sav"}},
	"SNES":      {Rom: []string{".smc", ".sfc"}, Save: []string{".srm", ".sav"}},
	"Mega Drive": {Rom: []string{".md", ".bin", ".gen", ".smd"}, Save: []string{".srm", ".sav"}},
	"Saturn":    {Rom: []string{".iso", ".bin", ".cue", ".mdf", ".chd"}, Save: []string{".bkr", ".srm", ".sav"}},
	"N64":       {Rom: []string{".z64", ".n64", ".v64"}, Save: []string{".sra", ".fla", ".eep", ".mpk", ".srm"}},
	"Dreamcast": {Rom: []string{".gdi", ".cdi", ".iso", ".chd"}, Save: []string{".vmu", ".bin"}},
	"PS1":       {Rom: []string{".iso", ".bin", ".cue", ".img", ".chd"}, Save: []string{".mcr", ".mcd", ".srm", ".mem"}},
	"PS2":       {Rom: []string{".iso", ".bin", ".chd"}, Save: []string{".ps2", ".mcr", ".xps", ".max", ".psu"}},
	"Switch":    {Rom: []string{".nsp", ".xci", ".nsz"}, Save: []string{".sav", ".bin"}},
}

type Server struct {
	mu            sync.RWMutex
	cfg           HostConfig
	configPath    string
	webDir        string
	fileTypesPath string
	scanPathsPath string
}

func main() {
	addr := flag.String("addr", ":5035", "listen address")
	configPath := flag.String("config", "hostconfig.json", "host config path")
	webDir := flag.String("web", "web", "web files directory")
	flag.Parse()

	stateDir := filepath.Dir(*configPath)
	s := &Server{
		configPath:    *configPath,
		webDir:        *webDir,
		fileTypesPath: filepath.Join(stateDir, "filetypes.json"),
		scanPathsPath: filepath.Join(stateDir, "scanpaths.json"),
	}
	if data, err := os.ReadFile(*configPath); err == nil {
		json.Unmarshal(data, &s.cfg)
	}
	if _, err := os.Stat(s.fileTypesPath); os.IsNotExist(err) {
		if data, err := json.MarshalIndent(defaultFileTypes, "", "  "); err == nil {
			os.WriteFile(s.fileTypesPath, data, 0644)
		}
	}
	if _, err := os.Stat(s.scanPathsPath); os.IsNotExist(err) {
		if data, err := json.MarshalIndent(defaultScanPaths, "", "  "); err == nil {
			os.WriteFile(s.scanPathsPath, data, 0644)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/consoles", s.lanOnly(s.handleConsoles))
	mux.HandleFunc("/api/files", s.lanOnly(s.handleFiles))
	mux.HandleFunc("/api/download", s.lanOnly(s.handleDownload))
	mux.HandleFunc("/api/hostconfig", s.lanOnly(s.handleHostConfig))
	mux.HandleFunc("/api/saves", s.lanOnly(s.handleSaves))
	mux.HandleFunc("/api/saves/download", s.lanOnly(s.handleSavesDownload))
	mux.HandleFunc("/api/saves/upload", s.lanOnly(s.handleSavesUpload))
	mux.HandleFunc("/api/filetypes", s.lanOnly(s.handleFileTypes))
	mux.HandleFunc("/api/scanpaths", s.lanOnly(s.handleScanPaths))
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := []SaveFileInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		game := ""
		sidecarPath := filepath.Join(dir, strings.TrimSuffix(name, filepath.Ext(name))+".json")
		if data, err := os.ReadFile(sidecarPath); err == nil {
			var sc struct {
				Game string `json:"game"`
			}
			if json.Unmarshal(data, &sc) == nil {
				game = sc.Game
			}
		}
		result = append(result, SaveFileInfo{Name: name, Size: info.Size(), Game: game})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	stamped := base + "_" + time.Now().Format("20060102-150405") + ext
	dest := filepath.Join(dir, stamped)
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
	if game := r.URL.Query().Get("game"); game != "" {
		type sidecar struct {
			Game         string `json:"game"`
			Console      string `json:"console"`
			PushedAt     string `json:"pushed_at"`
			OriginalFile string `json:"original_file"`
		}
		sc := sidecar{
			Game:         game,
			Console:      console,
			PushedAt:     time.Now().Format(time.RFC3339),
			OriginalFile: file,
		}
		if data, err := json.Marshal(sc); err == nil {
			sidecarPath := strings.TrimSuffix(dest, filepath.Ext(dest)) + ".json"
			os.WriteFile(sidecarPath, data, 0644)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFileTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var ft map[string]ConsoleFileTypes
		if err := json.NewDecoder(r.Body).Decode(&ft); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := json.MarshalIndent(ft, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(s.fileTypesPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ft := make(map[string]ConsoleFileTypes)
	for k, v := range defaultFileTypes {
		ft[k] = v
	}
	if data, err := os.ReadFile(s.fileTypesPath); err == nil {
		var loaded map[string]ConsoleFileTypes
		if json.Unmarshal(data, &loaded) == nil {
			for k, v := range loaded {
				ft[k] = v
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ft)
}

func (s *Server) handleScanPaths(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var paths []ScanPath
		if err := json.NewDecoder(r.Body).Decode(&paths); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := json.MarshalIndent(paths, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(s.scanPathsPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var paths []ScanPath
	if data, err := os.ReadFile(s.scanPathsPath); err == nil {
		json.Unmarshal(data, &paths)
	}
	if paths == nil {
		paths = defaultScanPaths
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(paths)
}
