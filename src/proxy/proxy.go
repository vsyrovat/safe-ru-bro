package proxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"safe-ru-bro/src/config"
)

const (
	kindSOCKS5 = "socks5"
	kindHTTP   = "http"
)

// Listen serves a local HTTP proxy that forwards to upstream. addr is what
// Chromium should use as --proxy-server, without a scheme.
func Listen(bind string, upstream config.Proxy) (addr string, stop func(), err error) {
	kind, err := detect(upstream)
	if err != nil {
		return "", nil, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(bind, "0"))
	if err != nil {
		return "", nil, err
	}
	go serve(listener, upstream, kind)
	return listener.Addr().String(), func() { listener.Close() }, nil
}

func serve(listener net.Listener, upstream config.Proxy, kind string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go handle(conn, upstream, kind)
	}
}

func handle(conn net.Conn, upstream config.Proxy, kind string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	dest, err := target(req)
	if err != nil {
		io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		return
	}
	remote, err := dial(upstream, kind, dest)
	if err != nil {
		io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer remote.Close()
	if req.Method == http.MethodConnect {
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
	} else {
		req.RequestURI = ""
		req.Header.Del("Proxy-Connection")
		if err := req.Write(remote); err != nil {
			return
		}
	}
	splice(conn, reader, remote)
}

func target(req *http.Request) (string, error) {
	host := req.Host
	if req.Method != http.MethodConnect && req.URL != nil && req.URL.Host != "" {
		host = req.URL.Host
	}
	if host == "" {
		return "", fmt.Errorf("missing host")
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		port := "80"
		if req.Method == http.MethodConnect {
			port = "443"
		}
		host = net.JoinHostPort(host, port)
	}
	return host, nil
}

func splice(client net.Conn, reader *bufio.Reader, remote net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(remote, reader)
		remote.Close()
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, remote)
		client.Close()
		done <- struct{}{}
	}()
	<-done
}

func dial(upstream config.Proxy, kind, dest string) (net.Conn, error) {
	switch kind {
	case kindSOCKS5:
		return dialSOCKS5(upstream, dest)
	default:
		return dialHTTP(upstream, dest)
	}
}

func detect(upstream config.Proxy) (string, error) {
	conn, err := net.DialTimeout("tcp", upstream.Address(), 8*time.Second)
	if err != nil {
		return "", fmt.Errorf("proxy %s: %w", upstream.Address(), err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))
	methods := []byte{0x05, 0x01, 0x00}
	if upstream.User != "" || upstream.Pass != "" {
		methods = []byte{0x05, 0x01, 0x02}
	}
	if _, err := conn.Write(methods); err != nil {
		return kindHTTP, nil
	}
	var reply [2]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil || reply[0] != 0x05 {
		return kindHTTP, nil
	}
	if reply[1] == 0x02 || reply[1] == 0x00 {
		return kindSOCKS5, nil
	}
	return kindHTTP, nil
}

func dialSOCKS5(upstream config.Proxy, dest string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", upstream.Address(), 20*time.Second)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	if err := socks5Handshake(conn, upstream, dest); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func socks5Handshake(conn net.Conn, upstream config.Proxy, dest string) error {
	greeting := []byte{0x05, 0x01, 0x00}
	if upstream.User != "" || upstream.Pass != "" {
		greeting[2] = 0x02
	}
	if _, err := conn.Write(greeting); err != nil {
		return err
	}
	var method [2]byte
	if _, err := io.ReadFull(conn, method[:]); err != nil {
		return err
	}
	if method[0] != 0x05 {
		return fmt.Errorf("socks5: unexpected version %d", method[0])
	}
	if method[1] == 0x02 {
		if err := socks5Auth(conn, upstream); err != nil {
			return err
		}
	} else if method[1] != 0x00 {
		return fmt.Errorf("socks5: authentication rejected")
	}
	host, port, err := net.SplitHostPort(dest)
	if err != nil {
		return err
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	if len(host) > 255 {
		return fmt.Errorf("socks5: host name too long")
	}
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, host...)
	req = append(req, byte(portNum>>8), byte(portNum))
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return err
	}
	if head[1] != 0x00 {
		return fmt.Errorf("socks5: connect failed (%d)", head[1])
	}
	return discardBindAddr(conn, head[3])
}

func socks5Auth(conn net.Conn, upstream config.Proxy) error {
	user := []byte(upstream.User)
	pass := []byte(upstream.Pass)
	if len(user) > 255 || len(pass) > 255 {
		return fmt.Errorf("socks5: credentials too long")
	}
	buf := []byte{0x01, byte(len(user))}
	buf = append(buf, user...)
	buf = append(buf, byte(len(pass)))
	buf = append(buf, pass...)
	if _, err := conn.Write(buf); err != nil {
		return err
	}
	var reply [2]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return err
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("socks5: authentication failed")
	}
	return nil
}

func discardBindAddr(conn net.Conn, atyp byte) error {
	switch atyp {
	case 0x01:
		_, err := io.CopyN(io.Discard, conn, 4+2)
		return err
	case 0x04:
		_, err := io.CopyN(io.Discard, conn, 16+2)
		return err
	case 0x03:
		var n [1]byte
		if _, err := io.ReadFull(conn, n[:]); err != nil {
			return err
		}
		_, err := io.CopyN(io.Discard, conn, int64(n[0])+2)
		return err
	default:
		return fmt.Errorf("socks5: unknown address type %d", atyp)
	}
}

func dialHTTP(upstream config.Proxy, dest string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", upstream.Address(), 20*time.Second)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	var auth string
	if upstream.User != "" || upstream.Pass != "" {
		token := base64.StdEncoding.EncodeToString([]byte(upstream.User + ":" + upstream.Pass))
		auth = "Proxy-Authorization: Basic " + token + "\r\n"
	}
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n%sProxy-Connection: keep-alive\r\n\r\n", dest, dest, auth)
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("http proxy connect: %s", resp.Status)
	}
	conn.SetDeadline(time.Time{})
	return &prefixConn{Conn: conn, reader: reader}, nil
}

type prefixConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *prefixConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
