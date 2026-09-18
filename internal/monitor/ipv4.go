package monitor

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"strings"
)

// doggo's HTTP/2 transport does not use its --ipv4 option. A concrete local
// IPv4 address forces Go's TCP address-family filtering; 0.0.0.0 would be a
// wildcard and would NOT provide that guarantee. UDP dialing chooses a route
// without sending any packet; hostname resolution explicitly uses udp4.
func ipv4Source(ctx context.Context, upstream string) (string, error) {
	u, err := url.Parse(commandAddress(upstream))
	if err != nil {
		return "", err
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp4", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP.To4() == nil || local.IP.IsUnspecified() {
		return "", errors.New("无法选择可用 IPv4 本地地址")
	}
	return local.IP.To4().String(), nil
}

// Translate simple plain/DoT/DoQ stamps into native doggo URLs. Its stamp
// dispatcher only implements DNSCrypt and DoH. Unsupported pin/bootstrap
// constraints are rejected by ValidateAddress rather than silently discarded.
func commandAddress(address string) string {
	if !strings.HasPrefix(address, "sdns://") {
		return address
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(address, "sdns://"))
	if err != nil || len(b) < 10 {
		return address
	}
	p := b[0]
	if p != 0 && p != 2 && p != 3 && p != 4 {
		return address
	}
	pos := 9
	read := func() (string, bool) {
		if pos >= len(b) {
			return "", false
		}
		n := int(b[pos])
		pos++
		if pos+n > len(b) {
			return "", false
		}
		s := string(b[pos : pos+n])
		pos += n
		return s, true
	}
	addr, ok := read()
	if !ok {
		return address
	}
	if p == 0 {
		return "udp://" + addr
	}
	// Validation permits empty hashes only. The stamp VLP encoding may still
	// contain more than one empty entry, so consume all continuation markers.
	for {
		if pos >= len(b) || b[pos]&127 != 0 {
			return address
		}
		more := b[pos]&128 != 0
		pos++
		if !more {
			break
		}
	}
	host, ok := read()
	if !ok {
		return address
	}
	if p == 2 {
		path, ok := read()
		if !ok {
			return address
		}
		return (&url.URL{Scheme: "https", Host: host, Path: path}).String()
	}
	scheme := "tls://"
	if p == 4 {
		scheme = "quic://"
	}
	return scheme + host
}

func probeEnvironment(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if strings.HasPrefix(key, "DOGGO_") {
			continue
		}
		switch key {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		}
		out = append(out, entry)
	}
	return out
}
