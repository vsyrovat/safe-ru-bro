package main

import "testing"

func TestSplitArgs(t *testing.T) {
	noProxy, urls := splitArgs([]string{"https://example.com", "--no-proxy", "https://other.example"})
	if !noProxy {
		t.Fatal("no-proxy was not set")
	}
	if len(urls) != 2 || urls[0] != "https://example.com" || urls[1] != "https://other.example" {
		t.Fatalf("%q", urls)
	}

	noProxy, urls = splitArgs([]string{"https://example.com"})
	if noProxy || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("noProxy %v urls %q", noProxy, urls)
	}
}
