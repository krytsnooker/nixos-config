package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr = ":5030"
	mjhBase    = "https://i.mjh.nz/.r/"
	userAgent  = "AppleTV/tvOS/9.1.1 Darwin/15.2.0"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

var noRedirectClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// --- channel session state ---

type variant struct {
	attrs string
	url   string
}

type channelState struct {
	mu          sync.Mutex
	variants    []variant
	refreshedAt time.Time
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
	s := &channelState{}
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

// --- URL helpers ---

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

// --- handlers ---

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

func fetchVariantPlaylist(cs *channelState, channel string, idx int) (string, error) {
	varURL, ok := cs.variantURL(idx)
	if !ok {
		return "", fmt.Errorf("variant %d not found for %s", idx, channel)
	}

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

	return rewriteMediaPlaylist(string(b), baseOf(varURL)), nil
}

func rewriteMediaPlaylist(body, base string) string {
	var sb strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
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
			fmt.Fprintf(w, "%s/stream/%s.m3u8\n", proxyBase, channel)
		} else {
			fmt.Fprintln(w, line)
		}
	}
}

func main() {
	http.HandleFunc("/stream/", streamHandler)
	http.HandleFunc("/hls/", hlsHandler)
	http.HandleFunc("/playlist.m3u8", playlistHandler)

	log.Printf("mjh-proxy listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
