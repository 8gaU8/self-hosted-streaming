package app

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var statusClient = &http.Client{Timeout: 2 * time.Second}

type statusEntry struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Up   bool   `json:"up"`
}

func serviceURL(svc Service, path string) string {
	return "http://" + net.JoinHostPort(svc.Host, strconv.Itoa(svc.Port)) + path
}

func checkService(svc Service) bool {
	resp, err := statusClient.Get(serviceURL(svc, svc.CheckPath))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	results := make([]statusEntry, len(linkedServices))
	var wg sync.WaitGroup
	for i, svc := range linkedServices {
		wg.Add(1)
		go func(i int, svc Service) {
			defer wg.Done()
			results[i] = statusEntry{Key: svc.Key, Name: svc.Name, Up: checkService(svc)}
		}(i, svc)
	}
	wg.Wait()

	writeJSON(w, results)
}

func usageHandler(w http.ResponseWriter, r *http.Request, collector *DockerCollector) {
	writeJSON(w, collector.usage(r.Context()))
}

var iconLinkRe = regexp.MustCompile(`(?i)<link[^>]+rel=["'](?:shortcut icon|icon)["'][^>]*href=["']([^"']+)["']`)

var (
	iconCacheMu sync.Mutex
	iconCache   = map[string]string{}
)

var iconClient = &http.Client{Timeout: 3 * time.Second}

func resolveIconURL(svc Service) (string, bool) {
	iconCacheMu.Lock()
	if cached, ok := iconCache[svc.Key]; ok {
		iconCacheMu.Unlock()
		return cached, true
	}
	iconCacheMu.Unlock()

	resp, err := iconClient.Get(serviceURL(svc, svc.IconPage))
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			resp.Body.Close()
		}
		return "", false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}

	match := iconLinkRe.FindSubmatch(body)
	if match == nil {
		return "", false
	}

	// The href is relative to IconPage, which the proxy passes through
	// 1:1 (no path rewriting), so resolving it against that path also
	// gives the public URL.
	base, err := url.Parse(svc.IconPage)
	if err != nil {
		return "", false
	}
	ref, err := url.Parse(string(match[1]))
	if err != nil {
		return "", false
	}
	iconURL := base.ResolveReference(ref).String()

	iconCacheMu.Lock()
	iconCache[svc.Key] = iconURL
	iconCacheMu.Unlock()

	return iconURL, true
}

func iconHandler(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/icon/")

	var svc Service
	found := false
	for _, s := range linkedServices {
		if s.Key == key {
			svc, found = s, true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	iconURL, ok := resolveIconURL(svc)
	if !ok {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, iconURL, http.StatusFound)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
