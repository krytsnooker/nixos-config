package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	listenAddr   = ":5030"
	mjhBase      = "https://i.mjh.nz/.r/"
	userAgent    = "AppleTV/tvOS/9.1.1 Darwin/15.2.0"
	hlsWindow    = 12 // segments (~60s at 5s each)
	staleLimit   = 15 * time.Second
	pollInterval = 2 * time.Second
)

var (
	ffmpegPath = "/usr/bin/ffmpeg"
	runtimeDir = "/tmp"
	httpClient = &http.Client{Timeout: 15 * time.Second}
)

// isAdSegment returns true if the segment URL is from Google DAI ad/slate delivery.
func isAdSegment(u string) bool {
	return strings.Contains(u, "dai.google.com") || strings.Contains(u, "googlevideo.com")
}

// --- manifest filter ---

type channelFilter struct {
	mu        sync.Mutex
	channel   string
	mediaURL  string
	seqMap    map[string]int64 // segment URL → our filtered sequence number
	nextSeq   int64
	content   string // latest filtered playlist
	readyCh   chan struct{}
	readyOnce sync.Once
}

var (
	filtersMu sync.Mutex
	filters   = map[string]*channelFilter{}
)

func getFilter(channel string) *channelFilter {
	filtersMu.Lock()
	defer filtersMu.Unlock()
	if cf, ok := filters[channel]; ok {
		return cf
	}
	cf := &channelFilter{
		channel: channel,
		seqMap:  make(map[string]int64),
		readyCh: make(chan struct{}),
	}
	filters[channel] = cf
	go cf.pollLoop()
	return cf
}

func (cf *channelFilter) pollLoop() {
	for {
		cf.refresh()
		time.Sleep(pollInterval)
	}
}

func (cf *channelFilter) refresh() {
	cf.mu.Lock()
	mediaURL := cf.mediaURL
	cf.mu.Unlock()

	if mediaURL == "" {
		var err error
		mediaURL, err = cf.resolveMediaURL()
		if err != nil {
			log.Printf("filter %s: resolve: %v", cf.channel, err)
			return
		}
		log.Printf("filter %s: media playlist: %s", cf.channel, mediaURL)
		cf.mu.Lock()
		cf.mediaURL = mediaURL
		cf.mu.Unlock()
	}

	body, err := cf.fetchURL(mediaURL)
	if err != nil {
		log.Printf("filter %s: fetch media: %v — re-resolving", cf.channel, err)
		cf.mu.Lock()
		cf.mediaURL = ""
		cf.mu.Unlock()
		return
	}

	filtered, err := cf.buildFiltered(body)
	if err != nil {
		log.Printf("filter %s: build: %v", cf.channel, err)
		return
	}

	cf.mu.Lock()
	cf.content = filtered
	cf.mu.Unlock()
	cf.readyOnce.Do(func() { close(cf.readyCh) })
}

func (cf *channelFilter) fetchURL(rawURL string) (string, error) {
	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// resolveMediaURL fetches the mjh master playlist and returns the highest-bandwidth variant URL.
func (cf *channelFilter) resolveMediaURL() (string, error) {
	masterURL := mjhBase + cf.channel + ".m3u8"
	body, err := cf.fetchURL(masterURL)
	if err != nil {
		return "", fmt.Errorf("master: %w", err)
	}
	if strings.Contains(body, "#EXTINF") {
		return masterURL, nil // already a media playlist
	}
	bestBW := -1
	bestURL := ""
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			bw := parseBandwidth(line)
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next != "" && !strings.HasPrefix(next, "#") && bw > bestBW {
					bestBW = bw
					bestURL = next
				}
			}
		}
	}
	if bestURL == "" {
		return "", fmt.Errorf("no variant found in master playlist")
	}
	if !strings.HasPrefix(bestURL, "http") {
		return "", fmt.Errorf("unexpected relative variant URL: %s", bestURL)
	}
	return bestURL, nil
}

func parseBandwidth(streamInf string) int {
	for _, part := range strings.Split(streamInf, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "BANDWIDTH=") {
			var bw int
			fmt.Sscanf(strings.TrimPrefix(part, "BANDWIDTH="), "%d", &bw)
			return bw
		}
	}
	return 0
}

// buildFiltered parses the upstream media playlist, drops ad/slate segments and
// the #EXT-X-DISCONTINUITY markers on their boundaries, and returns a clean playlist
// with our own monotonically-increasing sequence numbers.
func (cf *channelFilter) buildFiltered(body string) (string, error) {
	type seg struct {
		keyLine  string // #EXT-X-KEY line (may be empty)
		extinf   string // #EXTINF line
		url      string
		discPre  bool // #EXT-X-DISCONTINUITY was immediately before this segment
	}

	var segs []seg
	var targetDuration string
	var pendingKey, pendingExtinf string
	var pendingDisc bool

	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "#EXT-X-TARGETDURATION:"):
			targetDuration = t
		case t == "#EXT-X-DISCONTINUITY":
			pendingDisc = true
		case strings.HasPrefix(t, "#EXT-X-KEY:"):
			pendingKey = t
		case strings.HasPrefix(t, "#EXTINF:"):
			pendingExtinf = t
		case pendingExtinf != "" && t != "" && !strings.HasPrefix(t, "#"):
			// This line is the segment URL following an #EXTINF
			segs = append(segs, seg{
				keyLine: pendingKey,
				extinf:  pendingExtinf,
				url:     t,
				discPre: pendingDisc,
			})
			pendingKey = ""
			pendingExtinf = ""
			pendingDisc = false
		}
	}

	// Assign our sequence numbers to non-ad segments (first time seen).
	cf.mu.Lock()
	for i := range segs {
		if !isAdSegment(segs[i].url) {
			if _, ok := cf.seqMap[segs[i].url]; !ok {
				cf.seqMap[segs[i].url] = cf.nextSeq
				cf.nextSeq++
			}
		}
	}
	firstSeq := int64(0)
	for _, s := range segs {
		if !isAdSegment(s.url) {
			firstSeq = cf.seqMap[s.url]
			break
		}
	}
	cf.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:3\n")
	if targetDuration != "" {
		sb.WriteString(targetDuration + "\n")
	}
	sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", firstSeq))

	prevWasAd := false
	for _, s := range segs {
		if isAdSegment(s.url) {
			prevWasAd = true
			continue
		}
		// Keep a content-to-content discontinuity, but not an ad-boundary one.
		if s.discPre && !prevWasAd {
			sb.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		prevWasAd = false
		if s.keyLine != "" {
			sb.WriteString(s.keyLine + "\n")
		}
		sb.WriteString(s.extinf + "\n")
		sb.WriteString(s.url + "\n")
	}

	return sb.String(), nil
}

func (cf *channelFilter) waitReady(timeout time.Duration) bool {
	select {
	case <-cf.readyCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (cf *channelFilter) getContent() string {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	return cf.content
}

func filteredHandler(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/filtered/"), ".m3u8")
	if channel == "" {
		http.Error(w, "bad path", 400)
		return
	}
	content := getFilter(channel).getContent()
	if content == "" {
		http.Error(w, "not ready", 503)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, content)
}

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
	// Point ffmpeg at our filtered manifest endpoint instead of upstream directly.
	filteredSrc := "http://localhost" + listenAddr + "/filtered/" + channel + ".m3u8"
	manifest := filepath.Join(ch.dir, "index.m3u8")
	segPattern := filepath.Join(ch.dir, "%06d.ts")

	if err := os.MkdirAll(ch.dir, 0755); err != nil {
		log.Printf("live %s: mkdir: %v", channel, err)
	}

	cf := getFilter(channel)
	log.Printf("live %s: waiting for first filtered manifest...", channel)
	if !cf.waitReady(30 * time.Second) {
		log.Printf("live %s: filter not ready after 30s, proceeding anyway", channel)
	}

	for {
		ctx, cancel := context.WithCancel(context.Background())

		cmd := exec.CommandContext(ctx, ffmpegPath,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "5",
			"-http_persistent", "0",
			"-user_agent", userAgent,
			"-i", filteredSrc,
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

		done := make(chan struct{})
		go func() {
			defer close(done)
			cmd.Wait()
		}()

		// Watchdog: if the local manifest stops updating, kill ffmpeg so it restarts.
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			lastMod := time.Now()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					if fi, err := os.Stat(manifest); err == nil && fi.ModTime().After(lastMod) {
						lastMod = fi.ModTime()
					}
					if time.Since(lastMod) > staleLimit {
						log.Printf("live %s: manifest stale for %s, restarting ffmpeg", channel, staleLimit)
						cancel()
						return
					}
				}
			}
		}()

		<-done
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

	http.HandleFunc("/filtered/", filteredHandler)
	http.HandleFunc("/live/", liveHandler)
	http.HandleFunc("/playlist.m3u8", playlistHandler)

	log.Printf("mjh-proxy listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
