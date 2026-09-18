package monitor

import (
	"dnsmonitor/internal/model"
	"net"
	"time"
)

func compareAt(result *model.ProbeResult, refs []model.Reference, now int64, historyHours float64) {
	result.References = refs
	result.Pollution = "unknown"
	result.Reason = "没有可用的可信 IPv4 参考"
	result.PolicyVersion = 2
	result.ComparedAt = now
	if !result.Success || result.Type != "A" || result.Rcode != "NOERROR" || len(result.Answers) == 0 {
		result.Reason = "目标查询未得到可比较的 IPv4 结果"
		return
	}
	fresh, allowed, prefixes := make(map[string]bool), make(map[string]bool), make(map[uint16]bool)
	for _, ref := range refs {
		if !ref.Success || ref.Rcode != "NOERROR" {
			continue
		}
		for _, record := range ref.Records {
			value, isFresh, usable := referenceEligibility(record, now, historyHours)
			if !usable {
				continue
			}
			if isFresh {
				fresh[value] = true
			}
			allowed[value] = true
			prefixes[ipv4Prefix(value)] = true
		}
	}
	if len(allowed) == 0 {
		return
	}
	verdict := "matched"
	for _, answer := range result.Answers {
		value := normalizeAnswer("A", answer)
		if value == "" {
			result.Reason = "目标包含无法比较的 IPv4 地址"
			return
		}
		if fresh[value] {
			continue
		}
		if allowed[value] {
			if verdict == "matched" {
				verdict = "clean"
			}
			continue
		}
		if !prefixes[ipv4Prefix(value)] {
			verdict = "polluted"
			break
		}
		verdict = "suspicious"
	}
	result.Pollution = verdict
	switch verdict {
	case "matched":
		result.Reason = "所有 IPv4 地址均命中未超过 TTL 的可信参考"
	case "clean":
		result.Reason = "所有 IPv4 地址均在可信参考池中，部分仅命中近期历史"
	case "suspicious":
		result.Reason = "出现可信参考池未见的 IPv4 地址，但前两段均能匹配参考池；规则评 E"
	case "polluted":
		result.Reason = "出现可信参考池未见且前两段无法匹配的 IPv4 地址；疑似污染，规则评 F，可手动撤销"
	}
}

func ipv4Prefix(value string) uint16 {
	ip := net.ParseIP(value).To4()
	return uint16(ip[0])<<8 | uint16(ip[1])
}

func referenceEligibility(record model.AnswerRecord, now int64, historyHours float64) (value string, fresh, usable bool) {
	value = normalizeAnswer("A", record.Value)
	if value == "" || record.ObservedAt <= 0 || record.ObservedAt > now {
		return value, false, false
	}
	fresh = record.TTLSeconds > 0 && record.ExpiresAt > now
	cutoff := now - int64(historyHours*float64(time.Hour/time.Millisecond))
	return value, fresh, fresh || (historyHours > 0 && record.ObservedAt > cutoff)
}

// Collection and verdict evaluation can straddle a TTL boundary. Negative
// evidence requires every collected source to remain eligible at ComparedAt,
// which is exactly the same instant used for all address/prefix membership.
func compareCollectionAt(result *model.ProbeResult, refs []model.Reference, complete bool, now int64, historyHours float64) {
	compareAt(result, refs, now, historyHours)
	if result.Pollution != "suspicious" && result.Pollution != "polluted" {
		return
	}
	for _, ref := range refs {
		usable := false
		if ref.Success && ref.Rcode == "NOERROR" {
			for _, record := range ref.Records {
				_, _, eligible := referenceEligibility(record, now, historyHours)
				if eligible {
					usable = true
					break
				}
			}
		}
		if !usable {
			complete = false
			break
		}
	}
	if !complete {
		result.Pollution = "unknown"
		result.Reason = "可信参考未完整采集或有来源参考已过期，暂不依据不完整集合评 E/F"
	}
}
