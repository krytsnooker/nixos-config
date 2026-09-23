package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr   = ":5030"
	mjhBase      = "https://i.mjh.nz/.r/"
	userAgent    = "AppleTV/tvOS/9.1.1 Darwin/15.2.0"
	stallTimeout = 20
)

var (
	ffmpegPath = "/usr/bin/ffmpeg"
	httpClient = &http.Client{Timeout: 15 * time.Second}

	noRedirectClient = &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

// --- channel session state (used by HLS proxy endpoints) ---

type variant struct {
	attrs string
	url   string
}

type channelState struct {
	mu          sync.Mutex
	variants    []variant
	refreshedAt time.Time
	seqs        map[int]int
}

var (
	statesMu sync.RWMutex
	states   = map[string]*channelState{}
)

func stateFor(channel string) *channelState {
	statesMu.Lock()
	defer statesMu.Unlock()
	if s, ok := states[channel]; ok {
		return s
	}
	s := &channelState{seqs: make(map[int]int)}
	states[channel] = s
	return s
}

func (cs *channelState) refresh(channel string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(cs.variants) > 0 && time.Since(cs.refreshedAt) < 5*time.Second {
		return nil
	}

	req, _ := http.NewRequest("GET", mjhBase+channel+".m3u8", nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return fmt.Errorf("mjh redirect: %w", err)
	}
	resp.Body.Close()

	daiURL := resp.Header.Get("Location")
	if daiURL == "" {
		return fmt.Errorf("no redirect from mjh for %s (status %d)", channel, resp.StatusCode)
	}

	req2, _ := http.NewRequest("GET", daiURL, nil)
	req2.Header.Set("User-Agent", userAgent)
	resp2, err := httpClient.Do(req2)
	if err != nil {
		return fmt.Errorf("fetch master: %w", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode >= 400 {
		return fmt.Errorf("master manifest HTTP %d", resp2.StatusCode)
	}

	base := baseOf(daiURL)
	variants, err := parseMaster(resp2.Body, base)
	if err != nil {
		return fmt.Errorf("parse master: %w", err)
	}
	if len(variants) == 0 {
		return fmt.Errorf("no variants in master manifest for %s", channel)
	}

	cs.variants = variants
	cs.seqs = make(map[int]int)
	cs.refreshedAt = time.Now()
	log.Printf("refreshed %s: %d variants", channel, len(variants))
	return nil
}

func (cs *channelState) variantURL(idx int) (string, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if idx < 0 || idx >= len(cs.variants) {
		return "", false
	}
	return cs.variants[idx].url, true
}

func (cs *channelState) getLastSeq(idx int) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	v, ok := cs.seqs[idx]
	if !ok {
		return -1
	}
	return v
}

func (cs *channelState) setLastSeq(idx, seq int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.seqs[idx] = seq
}

func parseMaster(r io.Reader, base string) ([]variant, error) {
	var variants []variant
	var attrs string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			attrs = line
		} else if attrs != "" && line != "" && !strings.HasPrefix(line, "#") {
			variants = append(variants, variant{
				attrs: attrs,
				url:   resolveURL(base, line),
			})
			attrs = ""
		}
	}
	return variants, scanner.Err()
}

func baseOf(rawURL string) string {
	if i := strings.LastIndex(rawURL, "/"); i >= 0 {
		return rawURL[:i+1]
	}
	return rawURL
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func parseMediaSeq(body string) int {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXT-X-MEDIA-SEQUENCE:") {
			v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#EXT-X-MEDIA-SEQUENCE:")))
			if err == nil {
				return v
			}
		}
	}
	return -1
}

// --- ffmpeg MPEG-TS handler ---

func mpegtsHandler(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.URL.Path, "/mpegts/")
	if channel == "" {
		http.Error(w, "missing channel", 400)
		return
	}

	src := mjhBase + channel + ".m3u8"
	log.Printf("mpegts %s: client connected", channel)

	w.Header().Set("Content-Type", "video/mp2t")

	for {
		if err := r.Context().Err(); err != nil {
			log.Printf("mpegts %s: client disconnected", channel)
			return
		}

		cmd := exec.CommandContext(r.Context(), ffmpegPath,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "5",
			"-user_agent", userAgent,
			"-i", src,
			"-c", "copy",
			"-f", "mpegts",
			"pipe:1",
		)
		cmd.Stderr = os.Stderr

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("mpegts %s: pipe error: %v, retrying in 2s", channel, err)
			time.Sleep(2 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			log.Printf("mpegts %s: start error: %v, retrying in 2s", channel, err)
			time.Sleep(2 * time.Second)
			continue
		}

		log.Printf("mpegts %s: ffmpeg started (pid %d)", channel, cmd.Process.Pid)
		_, copyErr := io.Copy(w, stdout)
		cmd.Wait()

		if r.Context().Err() != nil {
			log.Printf("mpegts %s: client disconnected", channel)
			return
		}

		log.Printf("mpegts %s: ffmpeg exited (copy err: %v), restarting in 1s", channel, copyErr)
		time.Sleep(1 * time.Second)
	}
}

// --- HLS proxy handlers (kept for reference/fallback) ---

func streamHandler(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/stream/"), ".m3u8")
	if channel == "" {
		http.Error(w, "missing channel", 400)
		return
	}

	cs := stateFor(channel)
	if err := cs.refresh(channel); err != nil {
		log.Printf("stream %s: %v", channel, err)
		http.Error(w, "stream unavailable", 502)
		return
	}

	cs.mu.Lock()
	variants := cs.variants
	cs.mu.Unlock()

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	proxyBase := scheme + "://" + r.Host

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	fmt.Fprintf(w, "#EXTM3U\n")
	for i, v := range variants {
		fmt.Fprintf(w, "%s\n%s/hls/%s/%d\n", v.attrs, proxyBase, channel, i)
	}
}

func hlsHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/hls/"), "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad path", 400)
		return
	}
	channel := parts[0]
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		http.Error(w, "bad variant index", 400)
		return
	}

	cs := stateFor(channel)

	body, err := fetchVariantPlaylist(cs, channel, idx)
	if err != nil {
		log.Printf("hls %s/%d: %v — refreshing", channel, idx, err)
		if rerr := cs.refresh(channel); rerr != nil {
			log.Printf("hls %s/%d: refresh failed: %v", channel, idx, rerr)
			http.Error(w, "stream unavailable", 502)
			return
		}
		body, err = fetchVariantPlaylist(cs, channel, idx)
		if err != nil {
			log.Printf("hls %s/%d: retry failed: %v", channel, idx, err)
			http.Error(w, "stream unavailable", 502)
			return
		}
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(body))
}

func getPlaylistBody(varURL string) (string, error) {
	req, _ := http.NewRequest("GET", varURL, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fetchVariantPlaylist(cs *channelState, channel string, idx int) (string, error) {
	varURL, ok := cs.variantURL(idx)
	if !ok {
		return "", fmt.Errorf("variant %d not found for %s", idx, channel)
	}

	body, err := getPlaylistBody(varURL)
	if err != nil {
		return "", err
	}

	seq := parseMediaSeq(body)
	lastSeq := cs.getLastSeq(idx)

	if seq != lastSeq || lastSeq == -1 {
		cs.setLastSeq(idx, seq)
		return rewriteMediaPlaylist(body, baseOf(varURL)), nil
	}

	log.Printf("playlist stalled for %s/%d at seq %d, waiting up to %ds", channel, idx, seq, stallTimeout)
	for i := 0; i < stallTimeout; i++ {
		time.Sleep(1 * time.Second)
		body, err = getPlaylistBody(varURL)
		if err != nil {
			return "", err
		}
		seq = parseMediaSeq(body)
		if seq != lastSeq {
			log.Printf("playlist resumed for %s/%d at seq %d (waited %ds)", channel, idx, seq, i+1)
			cs.setLastSeq(idx, seq)
			return rewriteMediaPlaylist(body, baseOf(varURL)), nil
		}
	}

	log.Printf("playlist stall timeout for %s/%d, returning stale playlist", channel, idx)
	return rewriteMediaPlaylist(body, baseOf(varURL)), nil
}

func rewriteMediaPlaylist(body, base string) string {
	var sb strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "#EXT-X-DISCONTINUITY" {
			continue
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			line = resolveURL(base, trimmed)
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// --- playlist proxy ---

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
			fmt.Fprintf(w, "%s/mpegts/%s\n", proxyBase, channel)
		} else {
			fmt.Fprintln(w, line)
		}
	}
}

func main() {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		ffmpegPath = p
	}
	log.Printf("using ffmpeg: %s", ffmpegPath)

	http.HandleFunc("/mpegts/", mpegtsHandler)
	http.HandleFunc("/stream/", streamHandler)
	http.HandleFunc("/hls/", hlsHandler)
	http.HandleFunc("/playlist.m3u8", playlistHandler)

	log.Printf("mjh-proxy listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
