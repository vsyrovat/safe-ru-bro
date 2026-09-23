package tunnel

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/engine"
)

// Run captures every packet on tun0 and forwards it to the SOCKS5 proxy,
// including UDP used by DNS and WebRTC.
func Run() error {
	host := os.Getenv("PROXY_HOST")
	port := os.Getenv("PROXY_PORT")
	if host == "" || port == "" {
		return fmt.Errorf("PROXY_HOST and PROXY_PORT are required")
	}
	proxyURL := &url.URL{
		Scheme: "socks5",
		Host:   net.JoinHostPort(host, port),
	}
	user, pass := os.Getenv("PROXY_USER"), os.Getenv("PROXY_PASS")
	if user != "" || pass != "" {
		proxyURL.User = url.UserPassword(user, pass)
	}

	engine.Insert(&engine.Key{
		Device:     "tun://tun0",
		Proxy:      proxyURL.String(),
		Interface:  "eth0",
		LogLevel:   "error",
		MTU:        1500,
		UDPTimeout: time.Minute,
		TUNPostUp:  postUp(host),
	})
	engine.Start()
	defer engine.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	return nil
}

func postUp(proxyHost string) string {
	// Chromium still opens IPv6 sockets when the kernel allows it, and this
	// proxy resets those handshakes. IPv6 stays off. The ClientHello also
	// does not fit in one segment on a 1500-byte path. Packets to the proxy
	// leave on the original interface, so that MTU stays at 1100.
	script := fmt.Sprintf(
		`ip addr add 198.18.0.1/15 dev tun0 && ip link set tun0 up && gw=$(ip -4 route show default | awk 'NR==1 {print $3}') && dev=$(ip -4 route show default | awk 'NR==1 {print $5}') && ip route replace %s via "$gw" dev "$dev" && ip link set "$dev" mtu 1100 && ip route replace default via 198.18.0.1 dev tun0`,
		strconv.Quote(proxyHost+"/32"),
	)
	return "sh -c " + strconv.Quote(script)
}
