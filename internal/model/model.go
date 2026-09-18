package model

import "time"

type Config struct {
	Listen              string   `json:"listen"`
	IntervalSeconds     int      `json:"interval_seconds"`
	TimeoutSeconds      int      `json:"timeout_seconds"`
	Concurrency         int      `json:"concurrency"`
	SmartBackoff        bool     `json:"smart_backoff"`
	MaxBackoffHours     float64  `json:"max_backoff_hours"`
	ReferenceTTLSeconds int      `json:"reference_ttl_seconds"`
	Domains             []Domain `json:"domains"`
}
type Domain struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func DefaultConfig() Config {
	return Config{Listen: "0.0.0.0:8080", IntervalSeconds: 300, TimeoutSeconds: 3, Concurrency: 2, SmartBackoff: true, MaxBackoffHours: 1, ReferenceTTLSeconds: 300, Domains: []Domain{{Name: "example.com", Type: "A"}, {Name: "cloudflare.com", Type: "A"}}}
}

type Server struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Address   string `json:"address"`
	Protocol  string `json:"protocol"`
	Enabled   bool   `json:"enabled"`
	Trusted   bool   `json:"trusted"`
	Notes     string `json:"notes"`
	CreatedAt int64  `json:"created_at"`
}
type Reference struct {
	ServerID  int64    `json:"server_id"`
	Address   string   `json:"address"`
	Timestamp int64    `json:"timestamp"`
	Rcode     string   `json:"rcode"`
	Answers   []string `json:"answers"`
	Success   bool     `json:"success"`
	Error     string   `json:"error,omitempty"`
	Raw       string   `json:"raw,omitempty"`
}
type ProbeResult struct {
	ID         int64       `json:"id"`
	RoundID    int64       `json:"round_id"`
	ServerID   int64       `json:"server_id"`
	Timestamp  int64       `json:"timestamp"`
	Domain     string      `json:"domain"`
	Type       string      `json:"type"`
	Received   bool        `json:"received"`
	Success    bool        `json:"success"`
	LatencyMS  float64     `json:"latency_ms"`
	Rcode      string      `json:"rcode"`
	Answers    []string    `json:"answers"`
	Error      string      `json:"error,omitempty"`
	Raw        string      `json:"raw,omitempty"`
	Pollution  string      `json:"pollution"`
	Reason     string      `json:"reason"`
	References []Reference `json:"references"`
	Override   string      `json:"override,omitempty"`
}
type Round struct {
	Auxiliary  bool          `json:"auxiliary"`
	ID         int64         `json:"id"`
	ServerID   int64         `json:"server_id"`
	StartedAt  int64         `json:"started_at"`
	FinishedAt int64         `json:"finished_at"`
	NextDue    int64         `json:"next_due"`
	Results    []ProbeResult `json:"results"`
}
type Override struct {
	ServerID  int64  `json:"server_id"`
	Domain    string `json:"domain"`
	Type      string `json:"type"`
	Verdict   string `json:"verdict"`
	Note      string `json:"note"`
	UpdatedAt int64  `json:"updated_at"`
}
type Metrics struct {
	Samples      int64   `json:"samples"`
	Availability float64 `json:"availability"`
	SuccessRate  float64 `json:"success_rate"`
	AverageMS    float64 `json:"average_ms"`
	P95MS        float64 `json:"p95_ms"`
	Coverage     float64 `json:"coverage"`
	Pollution    string  `json:"pollution"`
	Grade        string  `json:"grade"`
	Score        float64 `json:"score"`
}
type ServerSummary struct {
	Server
	Metrics     Metrics `json:"metrics"`
	LastProbe   int64   `json:"last_probe"`
	NextDue     int64   `json:"next_due"`
	LastSuccess bool    `json:"last_success"`
	Failures    int     `json:"failures"`
}
type HistoryPoint struct {
	Timestamp int64 `json:"timestamp"`
	Metrics
}
type RuntimeStatus struct {
	Active    int    `json:"active"`
	Paused    bool   `json:"paused"`
	LastError string `json:"last_error"`
	StartedAt int64  `json:"started_at"`
}

const Retention = 30 * 24 * time.Hour
