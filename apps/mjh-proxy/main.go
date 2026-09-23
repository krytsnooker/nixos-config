package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr = ":5030"
	mjhBase    = "https://i.mjh.nz/.r/"
	userAgent  = "AppleTV/tvOS/9.1.1 Darwin/15.2.0"
	hlsWindow  = 12 // segments (~60s at 5s each)
)

var (
	ffmpegPath = "/usr/bin/ffmpeg"
	runtimeDir = "/tmp"
	httpClient = &http.Client{Timeout: 15 * time.Second}
)

// --- live channel management ---

type liveChannel struct {
	mu      sync.Mutex
	running bool
	dir     string
}

var (
	livesMu sync.Mutex
	lives   = map[string]*liveChannel{}
)

func getLive(channel string) *liveChannel {
	livesMu.Lock()
	defer livesMu.Unlock()
	if ch, ok := lives[channel]; ok {
		return ch
	}
	ch := &liveChannel{
		dir: filepath.Join(runtimeDir, "live-"+channel),
	}
	lives[channel] = ch
	return ch
}

func (ch *liveChannel) start(channel string) {
	ch.mu.Lock()
	if ch.running {
		ch.mu.Unlock()
		return
	}
	ch.running = true
	ch.mu.Unlock()

	go ch.run(channel)
}

func (ch *liveChannel) run(channel string) {
	src := mjhBase + channel + ".m3u8"
	manifest := filepath.Join(ch.dir, "index.m3u8")
	segPattern := filepath.Join(ch.dir, "%06d.ts")

	if err := os.MkdirAll(ch.dir, 0755); err != nil {
		log.Printf("live %s: mkdir: %v", channel, err)
	}

	for {
		ctx, cancel := context.WithCancel(context.Background())

		cmd := exec.CommandContext(ctx, ffmpegPath,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "5",
			"-user_agent", userAgent,
			"-i", src,
			"-c", "copy",
			"-f", "hls",
			"-hls_time", "5",
			"-hls_list_size", fmt.Sprintf("%d", hlsWindow),
			"-hls_flags", "delete_segments+append_list+omit_endlist",
			"-hls_segment_filename", segPattern,
			manifest,
		)
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			log.Printf("live %s: start error: %v, retrying in 5s", channel, err)
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("live %s: ffmpeg started (pid %d)", channel, cmd.Process.Pid)
		cmd.Wait()
		cancel()
		log.Printf("live %s: ffmpeg exited, restarting in 2s", channel)
		time.Sleep(2 * time.Second)
	}
}

// waitForManifest polls until the manifest file exists, up to 30s.
func (ch *liveChannel) waitForManifest() bool {
	manifest := filepath.Join(ch.dir, "index.m3u8")
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(manifest); err == nil {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// --- handlers ---

func liveHandler(w http.ResponseWriter, r *http.Request) {
	// /live/{channel}/index.m3u8  or  /live/{channel}/000001.ts
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/live/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "bad path", 400)
		return
	}
	channel := parts[0]
	file := filepath.Base(parts[1]) // prevent path traversal

	ch := getLive(channel)
	ch.start(channel)

	filePath := filepath.Join(ch.dir, file)

	if strings.HasSuffix(file, ".m3u8") {
		if !ch.waitForManifest() {
			http.Error(w, "stream not ready", 503)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filePath)
		return
	}

	if strings.HasSuffix(file, ".ts") {
		// Wait briefly for segment to appear if it was just listed in the manifest
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(filePath); err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "max-age=60")
		http.ServeFile(w, r, filePath)
		return
	}

	http.Error(w, "not found", 404)
}

func playlistHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := httpClient.Get("https://i.mjh.nz/au/Melbourne/raw.m3u8")
	if err != nil {
		http.Error(w, "upstream error", 502)
		return
	}
	defer resp.Body.Close()

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	proxyBase := scheme + "://" + r.Host

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "https://i.mjh.nz/.r/") {
			channel := strings.TrimSuffix(strings.TrimPrefix(trimmed, "https://i.mjh.nz/.r/"), ".m3u8")
			fmt.Fprintf(w, "%s/live/%s/index.m3u8\n", proxyBase, channel)
		} else {
			fmt.Fprintln(w, line)
		}
	}
}

func main() {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		ffmpegPath = p
	}
	if d := os.Getenv("RUNTIME_DIR"); d != "" {
		runtimeDir = d
	}
	log.Printf("using ffmpeg: %s, runtime dir: %s", ffmpegPath, runtimeDir)

	http.HandleFunc("/live/", liveHandler)
	http.HandleFunc("/playlist.m3u8", playlistHandler)

	log.Printf("mjh-proxy listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
