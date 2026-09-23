package main

import (
	"embed"
	"fmt"
	"os"

	"path/filepath"

	"safe-ru-bro/src/brand"
	"safe-ru-bro/src/config"
	"safe-ru-bro/src/container"
	"safe-ru-bro/src/host"
	"safe-ru-bro/src/tunnel"
	"safe-ru-bro/src/xauth"
)

//go:embed Dockerfile entrypoint.sh
var imageFiles embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tunnel" {
		if err := tunnel.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "safe-ru-bro: %s\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "brand" {
		dir := ""
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		if err := brand.Locales(dir); err != nil {
			fmt.Fprintf(os.Stderr, "safe-ru-bro: %s\n", err)
			os.Exit(1)
		}
		return
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "safe-ru-bro: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	noProxy, urls := splitArgs(args)
	session, err := host.Detect()
	if err != nil {
		return err
	}
	cookie, cleanup, err := xauth.Write(session.Display)
	if err != nil {
		return err
	}
	defer cleanup()

	runner, err := container.New(imageFiles)
	if err != nil {
		return err
	}
	if err := runner.Build(); err != nil {
		return err
	}

	var selected *config.Proxy
	var flags []string
	if !noProxy {
		upstream, defined, err := config.Load(filepath.Join(session.Root, config.FileName))
		if err != nil {
			return err
		}
		if defined {
			selected = &upstream
			fmt.Fprintf(os.Stderr, "proxy %s\n", upstream.Address())
			flags = []string{
				// The tunnel accepts a TCP handshake before the proxy connects.
				// A refused IPv6 or a dropped QUIC attempt then wins that race
				// and the page never falls back to working IPv4 HTTPS.
				"--disable-ipv6",
				"--disable-quic",
				"--force-webrtc-ip-handling-policy=default_public_interface_only",
			}
		}
	}
	return runner.Run(session, cookie, selected, flags, urls)
}

// splitArgs takes --no-proxy for this process and leaves the rest for the browser.
func splitArgs(args []string) (noProxy bool, urls []string) {
	for _, arg := range args {
		if arg == "--no-proxy" {
			noProxy = true
			continue
		}
		urls = append(urls, arg)
	}
	return noProxy, urls
}
