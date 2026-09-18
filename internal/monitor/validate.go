package monitor

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"dnsmonitor/internal/model"
)

// ValidateAddress accepts a single AdGuard Home upstream endpoint. Routing
// directives describe resolver policy rather than a server and are rejected.
func ValidateAddress(address string) (canonical, protocol string, err error) {
	a := strings.TrimSpace(address)
	if a == "" || len(a) > 4096 || strings.IndexFunc(a, unicode.IsSpace) >= 0 {
		return "", "", errors.New("DNS 地址不能为空、包含空白或超过 4096 字节")
	}
	if strings.HasPrefix(a, "[/") || a == "#" {
		return "", "", errors.New("请填写单个上游地址；不支持 AdGuard 分流规则或系统 DNS 占位符")
	}
	if strings.HasPrefix(a, "sdns://") {
		p, e := validateStamp(strings.TrimPrefix(a, "sdns://"))
		return "sdns://" + strings.TrimRight(strings.TrimPrefix(a, "sdns://"), "="), p, e
	}
	if !strings.Contains(a, "://") {
		a = "udp://" + a
	}
	u, e := url.Parse(a)
	if e != nil || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", "", errors.New("DNS 地址格式无效；不支持凭据、URL 片段或不安全 TLS 参数")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	protocols := map[string]string{"udp": "UDP", "tcp": "TCP", "tls": "DoT", "https": "DoH", "h3": "DoH3", "quic": "DoQ"}
	p, ok := protocols[u.Scheme]
	if !ok {
		return "", "", errors.New("支持 UDP、TCP、tls://、https://、h3://、quic:// 和 sdns://")
	}
	if e = validateHost(u.Hostname()); e != nil {
		return "", "", e
	}
	if strings.Contains(u.Host, "[") {
		return "", "", errors.New("仅支持 IPv4，不支持 IPv6 地址")
	}
	if e = validatePort(u.Port()); e != nil {
		return "", "", e
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", "", errors.New("端口不能为空")
	}
	if u.Scheme != "https" && u.Scheme != "h3" && (u.Path != "" || u.RawQuery != "") {
		return "", "", errors.New("只有 HTTPS/HTTP3 上游可以包含路径和查询参数")
	}
	if u.Scheme == "udp" || u.Scheme == "tcp" {
		if ip := net.ParseIP(u.Hostname()); ip == nil || ip.To4() == nil {
			return "", "", errors.New("普通 UDP/TCP 上游请填写 IPv4 地址，避免依赖系统 DNS")
		}
	}
	if (u.Scheme == "https" || u.Scheme == "h3") && u.Path == "" {
		u.Path = "/dns-query"
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), p, nil
}

func validatePort(port string) error {
	if port == "" {
		return nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("端口范围必须为 1–65535")
	}
	return nil
}

func validateHost(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil || strings.Contains(host, ":") {
			return errors.New("仅支持 IPv4")
		}
		return nil
	}
	if strings.Contains(host, ":") || strings.Contains(host, "%") {
		return errors.New("仅支持 IPv4")
	}
	return validateDomainName(host)
}

func validateDomainName(name string) error {
	name = strings.TrimSuffix(name, ".")
	if name == "" || len(name) > 253 {
		return errors.New("域名长度应为 1–253 字节")
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("域名标签格式无效")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return errors.New("域名须使用 ASCII/Punycode，不得包含空白或 URL")
			}
		}
	}
	return nil
}

// ValidateDomain normalizes a configured query, and is also useful for manual verdicts.
func ValidateDomain(d *model.Domain) error {
	d.Name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d.Name), "."))
	d.Type = strings.ToUpper(strings.TrimSpace(d.Type))
	if d.Type == "" {
		d.Type = "A"
	}
	if err := validateDomainName(d.Name); err != nil {
		return err
	}
	// IPv6 transport and AAAA queries are intentionally outside this app's scope.
	switch d.Type {
	case "A", "CNAME", "MX", "NS", "TXT", "SOA", "SRV", "CAA", "HTTPS", "SVCB", "PTR":
		return nil
	}
	return errors.New("查询类型支持 A、CNAME、MX、NS、TXT、SOA、SRV、CAA、HTTPS、SVCB、PTR（仅 IPv4）")
}

func ValidateConfig(c *model.Config) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(c.Listen))
	if err != nil {
		return errors.New("监听地址请填写 IPv4:端口，例如 0.0.0.0:8080")
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || strings.Contains(host, ":") {
		return errors.New("监听地址必须为 IPv4")
	}
	if err := validatePort(port); err != nil || port == "" {
		return errors.New("监听端口范围必须为 1–65535")
	}
	c.Listen = net.JoinHostPort(ip.String(), port)
	if c.IntervalSeconds < 5 || c.IntervalSeconds > 86400 {
		return errors.New("探测周期范围为 5–86400 秒")
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 60 {
		return errors.New("超时范围为 1–60 秒")
	}
	if c.Concurrency < 0 || c.Concurrency > 50 {
		return errors.New("并发范围为 0–50，0 表示暂停")
	}
	if math.IsNaN(c.MaxBackoffHours) || math.IsInf(c.MaxBackoffHours, 0) || c.MaxBackoffHours < 0 || c.MaxBackoffHours > 24 {
		return errors.New("最大退避范围为 0–24 小时，0 表示禁用")
	}
	if c.ReferenceTTLSeconds < 0 || c.ReferenceTTLSeconds > 86400 {
		return errors.New("可信参考缓存范围为 0–86400 秒，0 表示不使用缓存")
	}
	if math.IsNaN(c.ReferenceHistoryHours) || math.IsInf(c.ReferenceHistoryHours, 0) || c.ReferenceHistoryHours < 0 || c.ReferenceHistoryHours > 720 {
		return errors.New("可信近期历史范围为 0–720 小时，0 表示关闭历史参考")
	}
	if c.RatingWindowMinutes < 5 || c.RatingWindowMinutes > 1440 {
		return errors.New("评级统计窗口范围为 5–1440 分钟")
	}
	if c.RatingMinSamples < 1 || c.RatingMinSamples > 100 {
		return errors.New("评级最低采样数范围为 1–100 次")
	}
	if c.RatingMinCoverageMinutes < 0 || c.RatingMinCoverageMinutes > 1440 {
		return errors.New("评级最低覆盖时间范围为 0–1440 分钟，0 表示不要求覆盖时间")
	}
	if c.RatingMinCoverageMinutes > c.RatingWindowMinutes {
		return errors.New("评级最低覆盖时间不能超过评级统计窗口")
	}
	if len(c.Domains) == 0 || len(c.Domains) > 100 {
		return errors.New("请配置 1–100 个探测域名")
	}
	seen := make(map[string]bool)
	for i := range c.Domains {
		if err := ValidateDomain(&c.Domains[i]); err != nil {
			return fmt.Errorf("域名 %d: %w", i+1, err)
		}
		key := c.Domains[i].Name + "/" + c.Domains[i].Type
		if seen[key] {
			return errors.New("探测域名与记录类型不能重复")
		}
		seen[key] = true
	}
	return nil
}

func validateStamp(encoded string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil || len(b) < 10 {
		return "", errors.New("DNS stamp 编码无效")
	}
	p := b[0]
	protocols := map[byte]string{0: "UDP", 1: "DNSCrypt", 2: "DoH", 3: "DoT", 4: "DoQ"}
	protocol, ok := protocols[p]
	if !ok {
		return "", errors.New("不支持此 DNS stamp 协议（不支持匿名中继）")
	}
	pos := 9
	readLP := func() (string, error) {
		if pos >= len(b) {
			return "", errors.New("DNS stamp 字段缺失")
		}
		n := int(b[pos])
		pos++
		if pos+n > len(b) {
			return "", errors.New("DNS stamp 字段截断")
		}
		s := string(b[pos : pos+n])
		pos += n
		return s, nil
	}
	addr, err := readLP()
	if err != nil {
		return "", err
	}
	if addr != "" && p >= 2 {
		return "", errors.New("doggo 不支持此 stamp 的固定 IP 约束；请改用 https://、tls:// 或 quic:// 上游地址")
	}
	if addr != "" {
		host := addr
		if strings.Contains(addr, ":") {
			if p >= 2 {
				return "", errors.New("加密 DNS stamp 的 IP 字段不得含端口；请将端口写在主机名字段")
			}
			var port string
			host, port, err = net.SplitHostPort(addr)
			if err != nil {
				return "", errors.New("DNS stamp 必须使用 IPv4 地址")
			}
			if err = validatePort(port); err != nil {
				return "", err
			}
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil || strings.Contains(host, ":") {
			return "", errors.New("DNS stamp 地址必须使用 IPv4")
		}
	} else if p < 2 {
		return "", errors.New("DNS stamp 缺少 IPv4 地址")
	}
	if p == 0 {
		if pos != len(b) {
			return "", errors.New("DNS stamp 存在多余字段")
		}
		return protocol, nil
	}
	if p == 1 {
		publicKey, e := readLP()
		if e != nil || len(publicKey) != 32 {
			return "", errors.New("DNSCrypt 公钥必须为 32 字节")
		}
		provider, e := readLP()
		if e != nil || provider == "" {
			return "", errors.New("DNSCrypt 提供商字段无效")
		}
		if pos != len(b) {
			return "", errors.New("DNS stamp 存在多余字段")
		}
		return protocol, nil
	}
	// Certificate hashes are VLP fields; bit 7 means another hash follows.
	for {
		if pos >= len(b) {
			return "", errors.New("DNS stamp 哈希字段缺失")
		}
		n := int(b[pos])
		pos++
		more := n&128 != 0
		n &= 127
		if pos+n > len(b) {
			return "", errors.New("DNS stamp 哈希截断")
		}
		if n != 0 {
			return "", errors.New("doggo 不支持 stamp 证书哈希固定；请使用系统证书验证的 https://、tls:// 或 quic:// 地址")
		}
		pos += n
		if !more {
			break
		}
	}
	host, e := readLP()
	if e != nil || validateStampHost(host) != nil {
		return "", errors.New("DNS stamp 主机名无效")
	}
	if p == 2 {
		path, e := readLP()
		if e != nil || !strings.HasPrefix(path, "/") {
			return "", errors.New("DoH stamp 缺少有效路径")
		}
	}
	// Optional bootstrap addresses also use VLP; reject IPv6 entries.
	for pos < len(b) {
		n := int(b[pos])
		pos++
		more := n&128 != 0
		n &= 127
		if pos+n > len(b) {
			return "", errors.New("DNS stamp bootstrap 截断")
		}
		host := string(b[pos : pos+n])
		pos += n
		if n > 0 {
			return "", errors.New("doggo 不支持 stamp bootstrap 约束；请使用标准上游地址")
		}
		if n > 0 {
			if ip := net.ParseIP(host); ip == nil || ip.To4() == nil || strings.Contains(host, ":") {
				return "", errors.New("DNS stamp bootstrap 必须为 IPv4")
			}
		}
		if more && pos == len(b) {
			return "", errors.New("DNS stamp bootstrap 后续字段缺失")
		}
		if !more && pos < len(b) {
			return "", errors.New("DNS stamp 存在多余字段")
		}
	}
	return protocol, nil
}

func validateStampHost(value string) error {
	host := value
	if strings.Contains(value, ":") {
		var port string
		var err error
		host, port, err = net.SplitHostPort(value)
		if err != nil {
			return err
		}
		if port == "" {
			return errors.New("stamp port is empty")
		}
		if err = validatePort(port); err != nil {
			return err
		}
	}
	return validateHost(host)
}
