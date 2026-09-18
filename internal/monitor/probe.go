package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
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
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address"`
	MName   string `json:"mname"`
	Status  string `json:"status"`
	RTT     string `json:"rtt"`
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
	result := model.ProbeResult{ServerID: server.ID, Timestamp: started.UnixMilli(), Domain: domain.Name, Type: domain.Type, Pollution: "unknown", Reason: "尚未与可信 DNS 比较", Answers: []string{}, References: []model.Reference{}}
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
	for _, record := range resp.Answers {
		if strings.EqualFold(record.Type, domain.Type) {
			value := normalizeAnswer(domain.Type, record.Address)
			if value != "" {
				r.Answers = append(r.Answers, value)
			}
		}
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

func compare(result *model.ProbeResult, refs []model.Reference) {
	result.References = refs
	result.Pollution = "unknown"
	result.Reason = "没有成功的可信 DNS 参考回答"
	if !result.Success {
		result.Reason = "目标查询未得到可比较结果"
		return
	}
	var expected string
	count := 0
	for _, ref := range refs {
		if !ref.Success || (ref.Rcode != "NOERROR" && ref.Rcode != "NXDOMAIN") || (ref.Rcode == "NOERROR" && len(ref.Answers) == 0) {
			continue
		}
		key := answerKey(ref.Rcode, ref.Answers)
		if count > 0 && key != expected {
			result.Pollution = "unknown"
			result.Reason = "可信 DNS 之间回答不一致，暂停自动定罪"
			return
		}
		expected = key
		count++
	}
	if count == 0 {
		return
	}
	if answerKey(result.Rcode, result.Answers) == expected {
		result.Pollution = "clean"
		result.Reason = fmt.Sprintf("与 %d 个成功可信 DNS 的回答一致", count)
		return
	}
	result.Pollution = "polluted"
	result.Reason = fmt.Sprintf("与 %d 个回答一致的可信 DNS 不同，按配置自动定罪；可通过域名菜单撤销", count)
}

func answerKey(rcode string, answers []string) string {
	a := append([]string(nil), answers...)
	sort.Strings(a)
	a = unique(a)
	return rcode + "\x00" + strings.Join(a, "\x00")
}
