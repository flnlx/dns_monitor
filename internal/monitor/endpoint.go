package monitor

import (
	"net/url"
	"strings"
)

// sameEndpoint recognizes explicit default-port aliases without collapsing
// different transports, HTTP paths, or non-default ports into one reference.
func sameEndpoint(a, b string) bool {
	canonical := func(address string) string {
		address = commandAddress(address)
		if !strings.Contains(address, "://") {
			address = "udp://" + address
		}
		u, err := url.Parse(address)
		if err != nil {
			return address
		}
		defaults := map[string]string{"udp": "53", "tcp": "53", "tls": "853", "quic": "853", "https": "443", "h3": "443"}
		if port, ok := defaults[u.Scheme]; ok && u.Port() == port {
			u.Host = u.Hostname()
		}
		return u.String()
	}
	return canonical(a) == canonical(b)
}
