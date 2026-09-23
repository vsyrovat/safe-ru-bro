package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

const FileName = "safe-ru-bro.conf"

// Proxy is an upstream HTTP or SOCKS5 endpoint. Defined is false when the
// config file is missing or the proxy line is empty.
type Proxy struct {
	Host string
	Port string
	User string
	Pass string
}

func (p Proxy) Address() string {
	return net.JoinHostPort(p.Host, p.Port)
}

func Load(path string) (Proxy, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Proxy{}, false, nil
		}
		return Proxy{}, false, err
	}
	defer file.Close()

	var raw string
	var found bool
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "proxy" {
			return Proxy{}, false, fmt.Errorf("%s: unknown setting %q", path, line)
		}
		raw = strings.TrimSpace(value)
		found = true
	}
	if err := scanner.Err(); err != nil {
		return Proxy{}, false, err
	}
	if !found || raw == "" {
		return Proxy{}, false, nil
	}
	proxy, err := parse(raw)
	if err != nil {
		return Proxy{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return proxy, true, nil
}

func parse(raw string) (Proxy, error) {
	parts := strings.SplitN(raw, ":", 4)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Proxy{}, fmt.Errorf("proxy must be host:port or host:port:user:password")
	}
	proxy := Proxy{Host: parts[0], Port: parts[1]}
	if len(parts) > 2 {
		proxy.User = parts[2]
	}
	if len(parts) > 3 {
		proxy.Pass = parts[3]
	}
	return proxy, nil
}
