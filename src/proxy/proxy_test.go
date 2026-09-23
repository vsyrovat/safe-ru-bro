package proxy

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"safe-ru-bro/src/config"
)

func TestConfiguredProxyReachesInternet(t *testing.T) {
	upstream, defined, err := config.Load(filepath.Join("..", "..", config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !defined {
		t.Skip("no proxy configured")
	}
	addr, stop, err := Listen("127.0.0.1", upstream)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: addr}),
		},
		Timeout: 25 * time.Second,
	}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %s body %s", resp.Status, body)
	}
	ip := strings.TrimSpace(string(body))
	if ip == "" {
		t.Fatal("empty address")
	}
	t.Logf("egress %s", ip)
}
