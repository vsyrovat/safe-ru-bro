# Safe Ru Browser

Ungoogled Chromium on the host display, inside a container. The profile, cache, and downloads stay in directories next to the `safe-ru-bro` binary. Window titles end with SafeRuBro.

The image is Debian Trixie with [Ungoogled Chromium 153.0.8010.52](https://github.com/ungoogled-software/ungoogled-chromium-portablelinux/releases/tag/153.0.8010.52-1). That portable build is published for amd64 only. The image also installs the [Russian trusted root and issuing CAs](https://status.tbank-online.com/certificates/#q7).

## Requirements

- Linux with an X11 `DISPLAY`
- Docker in `PATH`
- Go 1.27.1 or newer, to build the launcher

PulseAudio and `/dev/dri` are passed into the container when they exist on the host.

## Run

```sh
make build
./safe-ru-bro
./safe-ru-bro https://example.com
./safe-ru-bro --no-proxy
```

The first run builds the `safe-ru-bro` image, which downloads Ungoogled Chromium, and opens https://browserleaks.com. Later runs reuse that image and restore the previous tabs.

Override CPU performance tier starts at TIER0: UNKNOWN. The choice can be changed in the browser settings. Arguments after the command name are passed to the browser. `--no-proxy` connects directly and ignores the proxy in the config file. `DISPLAY` must be set. Quit from the window, or press Ctrl+C: the container is stopped with enough time for Chromium to write its tabs. A crash opens a new tab instead of restoring the last session, so a bad page cannot lock the browser in a loop.

## Proxy

Put `safe-ru-bro.conf` in the same directory as the binary. A missing file, or an empty `proxy` line, connects directly.

```
# host:port or host:port:user:password
proxy=10.1.2.3:1080:user:password
```

When `proxy` is set, IPv6 inside the container is disabled and the container default route goes through `tun0`. Every packet is forwarded to that SOCKS5 endpoint. The interface that reaches the proxy stays at a 1100-byte MTU so Chromium's TLS handshake is not reset. The proxy host itself stays on the normal route.

## Files next to the binary

| Path | Contents |
| --- | --- |
| `data/` | Profile: cookies, tabs, site storage |
| `cache/` | HTTP cache and GPU shader caches |
| `downloads/` | Files saved from the browser |
| `safe-ru-bro.conf` | Optional proxy. Not read from any other directory |
