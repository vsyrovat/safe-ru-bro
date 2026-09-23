package config

import "testing"

func TestParse(t *testing.T) {
	proxy, err := parse("10.1.2.3:9314:user:secret")
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Host != "10.1.2.3" || proxy.Port != "9314" || proxy.User != "user" || proxy.Pass != "secret" {
		t.Fatalf("%+v", proxy)
	}
}

func TestParsePasswordWithColon(t *testing.T) {
	proxy, err := parse("10.0.0.1:1:user:a:b")
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Pass != "a:b" {
		t.Fatalf("password %q", proxy.Pass)
	}
}
