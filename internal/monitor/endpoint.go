package monitor

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// AddressKey validates and identifies an upstream independently of spelling,
// default ports and supported stamp wrappers. HTTP paths and query strings,
// transport, DNSCrypt public keys and non-default ports remain significant.
func AddressKey(address string) (string, error) {
	canonical, _, err := ValidateAddress(address)
	if err != nil {
		return "", err
	}
	address = commandAddress(canonical)
	if strings.HasPrefix(address, "sdns://") {
		// Validated DNSCrypt stamps are the only stamps not translated above.
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(address, "sdns://"))
		if err != nil || len(b) < 10 || b[0] != 1 {
			return "", fmt.Errorf("无法规范化 DNS stamp")
		}
		pos := 9
		read := func() []byte {
			n := int(b[pos])
			pos++
			value := b[pos : pos+n]
			pos += n
			return value
		}
		endpoint, key, provider := string(read()), read(), string(read())
		if host, port, e := net.SplitHostPort(endpoint); e == nil {
			if n, e := strconv.Atoi(port); e == nil {
				if n == 443 {
					endpoint = host
				} else {
					endpoint = net.JoinHostPort(host, strconv.Itoa(n))
				}
			}
		}
		return fmt.Sprintf("dnscrypt://%s/%x/%s", endpoint, key, strings.ToLower(strings.TrimSuffix(provider, "."))), nil
	}
	u, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" {
		n, _ := strconv.Atoi(port)
		port = strconv.Itoa(n)
	}
	defaults := map[string]string{"udp": "53", "tcp": "53", "tls": "853", "quic": "853", "https": "443", "h3": "443"}
	if port == "" || port == defaults[u.Scheme] {
		u.Host = host
	} else {
		u.Host = net.JoinHostPort(host, port)
	}
	return u.String(), nil
}

func sameEndpoint(a, b string) bool {
	ka, errA := AddressKey(a)
	kb, errB := AddressKey(b)
	return errA == nil && errB == nil && ka == kb
}
