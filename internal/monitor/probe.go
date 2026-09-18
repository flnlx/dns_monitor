package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"dnsmonitor/internal/model"
)

const maxProbeOutput = 256 * 1024

// limitedBuffer drains the child process output while bounding memory usage.
type limitedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxProbeOutput - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

type doggoRecord struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Address string          `json:"address"`
	MName   string          `json:"mname"`
	Status  string          `json:"status"`
	RTT     string          `json:"rtt"`
	TTL     json.RawMessage `json:"ttl"`
}
type doggoOutput struct {
	Responses []struct {
		Answers     []doggoRecord `json:"answers"`
		Authorities []doggoRecord `json:"authorities"`
		Additional  []doggoRecord `json:"additional"`
		Questions   []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"questions"`
	} `json:"responses"`
	Error  string `json:"error"`
	Errors []struct {
		Error string `json:"error"`
	} `json:"errors"`
}

func doggoArgs(server model.Server, domain model.Domain, timeout time.Duration) []string {
	addr := commandAddress(server.Address)
	http3 := strings.HasPrefix(addr, "h3://")
	if http3 {
		addr = "https://" + strings.TrimPrefix(addr, "h3://")
	}
	args := []string{"--config=" + os.DevNull, "--query=" + domain.Name + ".", "--type=" + domain.Type, "--nameserver=" + addr, "--ipv4", "--json", "--color=false", "--search=false", "--ndots=0", "--timeout=" + timeout.String(), "--strategy=first"}
	if http3 {
		args = append(args, "--http3")
	}
	if http3 || strings.HasPrefix(addr, "quic://") {
		args = append(args, "--source=0.0.0.0")
	}
	return args
}

func runDoggo(ctx context.Context, path string, server model.Server, domain model.Domain, timeout time.Duration) model.ProbeResult {
	started := time.Now()
	result := model.ProbeResult{ServerID: server.ID, Timestamp: started.UnixMilli(), Domain: domain.Name, Type: domain.Type, PolicyVersion: 2, Pollution: "unknown", Reason: "尚未与可信 DNS 比较", Answers: []string{}, References: []model.Reference{}}
	// Hard deadline also covers resolver bootstrap and process startup. CommandContext
	// terminates the actual executable directly; no shell or command interpolation.
	probeCtx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()
	args := doggoArgs(server, domain, timeout)
	if strings.HasPrefix(commandAddress(server.Address), "https://") {
		source, err := ipv4Source(probeCtx, server.Address)
		if err != nil {
			result.Error = "IPv4 上游引导失败: " + err.Error()
			return result
		}
		args = append(args, "--source="+source)
	}
	cmd := exec.CommandContext(probeCtx, path, args...)
	configureCommand(cmd)
	cmd.WaitDelay = time.Second
	cmd.Env = probeEnvironment(os.Environ())
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result.Timestamp = time.Now().UnixMilli()
	result.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	result.Raw = stdout.String()
	if stderr.Len() > 0 {
		result.Raw += "\n[stderr]\n" + stderr.String()
	}
	if stdout.truncated || stderr.truncated {
		result.Error = "doggo 输出超过 256 KiB，结果不用于比较"
		return result
	}
	if probeCtx.Err() != nil {
		result.Error = probeCtx.Err().Error()
		return result
	}
	parsed, parseErr := decodeDoggo(stdout.Bytes(), domain)
	if parseErr != nil {
		result.Error = "无法解析 doggo JSON: " + parseErr.Error()
		if err != nil {
			result.Error = err.Error() + ": " + strings.TrimSpace(stderr.String())
		}
		return result
	}
	result.Received = parsed.Received
	result.Success = parsed.Success
	result.Rcode = parsed.Rcode
	result.Answers = parsed.Answers
	result.Records = parsed.Records
	if parsed.LatencyMS >= 0 {
		result.LatencyMS = parsed.LatencyMS
	}
	result.Error = parsed.Error
	if err != nil && result.Error == "" {
		result.Error = err.Error()
		result.Success = false
	}
	return result
}

func decodeDoggo(data []byte, domain model.Domain) (model.ProbeResult, error) {
	r := model.ProbeResult{Answers: []string{}, LatencyMS: -1, Rcode: "UNKNOWN"}
	var out doggoOutput
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&out); err != nil {
		return r, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return r, errors.New("JSON 后存在额外内容")
	}
	if len(out.Responses) != 1 {
		r.Error = out.Error
		if r.Error == "" && len(out.Errors) > 0 {
			r.Error = out.Errors[0].Error
		}
		if r.Error == "" {
			r.Error = "doggo 未返回唯一的 DNS 响应"
		}
		return r, nil
	}
	resp := out.Responses[0]
	if len(resp.Questions) != 1 || !strings.EqualFold(strings.TrimSuffix(resp.Questions[0].Name, "."), domain.Name) || !strings.EqualFold(resp.Questions[0].Type, domain.Type) {
		return r, errors.New("DNS 响应问题与请求不匹配")
	}
	r.Received = true
	for _, records := range [][]doggoRecord{resp.Answers, resp.Authorities, resp.Additional} {
		for _, record := range records {
			if record.Status != "" {
				r.Rcode = strings.ToUpper(record.Status)
			}
			if dt, e := time.ParseDuration(record.RTT); e == nil && dt >= 0 {
				r.LatencyMS = float64(dt.Microseconds()) / 1000
			}
		}
	}
	r.Timestamp = time.Now().UnixMilli()
	r.Records = requestedRecords(resp.Answers, domain, r.Timestamp)
	for _, record := range r.Records {
		r.Answers = append(r.Answers, record.Value)
	}
	sort.Strings(r.Answers)
	r.Answers = unique(r.Answers)
	// doggo v1.4 omits status on positive answers and omits the DNS header.
	// Positive requested records provide usable evidence; an empty header-only
	// response cannot distinguish SERVFAIL/REFUSED/NODATA and is never trusted.
	if r.Rcode == "UNKNOWN" && len(r.Answers) > 0 {
		r.Rcode = "NOERROR"
	}
	r.Success = (r.Rcode == "NOERROR" && len(r.Answers) > 0) || r.Rcode == "NXDOMAIN"
	if !r.Success {
		if r.Rcode == "UNKNOWN" {
			r.Error = "doggo 未提供响应码（空回答）；无法判断 SERVFAIL/REFUSED/NODATA"
		} else if r.Rcode == "NOERROR" {
			r.Error = "NODATA：没有请求类型的记录"
		} else {
			r.Error = r.Rcode
		}
	}
	if out.Error != "" {
		r.Error = out.Error
		r.Success = false
	}
	return r, nil
}

func normalizeAnswer(typ, value string) string {
	value = strings.TrimSpace(value)
	switch typ {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil {
			return ""
		}
		return ip.To4().String()
	case "CNAME", "NS", "PTR":
		return strings.ToLower(strings.TrimSuffix(value, "."))
	case "MX", "SRV":
		f := strings.Fields(value)
		if len(f) > 0 {
			f[len(f)-1] = strings.ToLower(strings.TrimSuffix(f[len(f)-1], "."))
		}
		return strings.Join(f, " ")
	case "SOA":
		f := strings.Fields(value)
		for i := 0; i < len(f) && i < 2; i++ {
			f[i] = strings.ToLower(strings.TrimSuffix(f[i], "."))
		}
		return strings.Join(f, " ")
	default:
		return value
	}
}

func unique(in []string) []string {
	if len(in) < 2 {
		return in
	}
	n := 1
	for i := 1; i < len(in); i++ {
		if in[i] != in[n-1] {
			in[n] = in[i]
			n++
		}
	}
	return in[:n]
}

// Unknown, fractional and negative TTLs never become fresh evidence. One day is
// a conservative upper bound against malformed upstream lifetimes, not a claim
// that DNS itself limits TTLs to one day.
func recordTTL(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		if d, err := time.ParseDuration(value); err == nil {
			if d <= 0 || d%time.Second != 0 {
				return 0
			}
			seconds := int64(d / time.Second)
			if seconds > 86400 {
				return 86400
			}
			return seconds
		}
	} else {
		value = string(raw)
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	if seconds > 86400 {
		return 86400
	}
	return seconds
}

func requestedRecords(answers []doggoRecord, domain model.Domain, now int64) []model.AnswerRecord {
	owner := strings.ToLower(strings.TrimSuffix(domain.Name, "."))
	pathTTL := int64(86400)
	seen := make(map[string]bool)
	for hops := 0; hops < 32; hops++ {
		if seen[owner] {
			return nil
		}
		seen[owner] = true
		alias, aliasTTL := "", int64(86400)
		if domain.Type != "CNAME" {
			for _, r := range answers {
				if !strings.EqualFold(strings.TrimSuffix(r.Name, "."), owner) || !strings.EqualFold(r.Type, "CNAME") {
					continue
				}
				next := normalizeAnswer("CNAME", r.Address)
				if next == "" || (alias != "" && alias != next) {
					return nil
				}
				alias = next
				if ttl := recordTTL(r.TTL); ttl < aliasTTL {
					aliasTTL = ttl
				}
			}
		}
		if alias != "" {
			if aliasTTL < pathTTL {
				pathTTL = aliasTTL
			}
			owner = alias
			continue
		}
		values := make(map[string]int64)
		for _, r := range answers {
			if !strings.EqualFold(strings.TrimSuffix(r.Name, "."), owner) || !strings.EqualFold(r.Type, domain.Type) {
				continue
			}
			value := normalizeAnswer(domain.Type, r.Address)
			if value == "" {
				continue
			}
			ttl := recordTTL(r.TTL)
			if pathTTL < ttl {
				ttl = pathTTL
			}
			if old, exists := values[value]; !exists || ttl < old {
				values[value] = ttl
			}
		}
		records := make([]model.AnswerRecord, 0, len(values))
		for value, ttl := range values {
			records = append(records, model.AnswerRecord{Value: value, TTLSeconds: ttl, ObservedAt: now, ExpiresAt: now + ttl*1000})
		}
		sort.Slice(records, func(i, j int) bool { return records[i].Value < records[j].Value })
		return records
	}
	return nil
}
