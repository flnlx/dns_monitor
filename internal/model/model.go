package model

import "time"

type Config struct {
	Listen                   string   `json:"listen"`
	IntervalSeconds          int      `json:"interval_seconds"`
	TimeoutSeconds           int      `json:"timeout_seconds"`
	Concurrency              int      `json:"concurrency"`
	SmartBackoff             bool     `json:"smart_backoff"`
	MaxBackoffHours          float64  `json:"max_backoff_hours"`
	ReferenceTTLSeconds      int      `json:"reference_ttl_seconds"`
	ReferenceHistoryHours    float64  `json:"reference_history_hours"`
	RatingWindowMinutes      int      `json:"rating_window_minutes"`
	RatingMinSamples         int      `json:"rating_min_samples"`
	RatingMinCoverageMinutes int      `json:"rating_min_coverage_minutes"`
	RatingWAvail             float64  `json:"rating_weight_availability"`
	RatingWSuccess           float64  `json:"rating_weight_success_rate"`
	RatingWLatency           float64  `json:"rating_weight_latency"`
	Domains                  []Domain `json:"domains"`
}
type Domain struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func DefaultConfig() Config {
	return Config{Listen: "0.0.0.0:8080", IntervalSeconds: 300, TimeoutSeconds: 3, Concurrency: 2, SmartBackoff: true, MaxBackoffHours: 1, ReferenceTTLSeconds: 300, ReferenceHistoryHours: 1, RatingWindowMinutes: 60, RatingMinSamples: 3, RatingMinCoverageMinutes: 5, RatingWAvail: .35, RatingWSuccess: .30, RatingWLatency: .35, Domains: []Domain{{Name: "www.youtube.com", Type: "A"}}}
}

type Server struct {
	TrustEpoch int64  `json:"-"`
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Address    string `json:"address"`
	Protocol   string `json:"protocol"`
	Enabled    bool   `json:"enabled"`
	Trusted    bool   `json:"trusted"`
	Notes      string `json:"notes"`
	CreatedAt  int64  `json:"created_at"`
}
type AnswerRecord struct {
	Value      string `json:"value"`
	TTLSeconds int64  `json:"ttl_seconds"`
	ObservedAt int64  `json:"observed_at"`
	ExpiresAt  int64  `json:"expires_at"`
}

type Reference struct {
	Records   []AnswerRecord `json:"records,omitempty"`
	ServerID  int64          `json:"server_id"`
	Address   string         `json:"address"`
	Timestamp int64          `json:"timestamp"`
	Rcode     string         `json:"rcode"`
	Answers   []string       `json:"answers"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Raw       string         `json:"raw,omitempty"`
}
type ProbeResult struct {
	ComparedAt         int64          `json:"compared_at,omitempty"`
	Records            []AnswerRecord `json:"records,omitempty"`
	PolicyVersion      int            `json:"policy_version,omitempty"`
	EffectivePollution string         `json:"effective_pollution,omitempty"`
	Trusted            bool           `json:"trusted"`
	ID                 int64          `json:"id"`
	RoundID            int64          `json:"round_id"`
	ServerID           int64          `json:"server_id"`
	Timestamp          int64          `json:"timestamp"`
	Domain             string         `json:"domain"`
	Type               string         `json:"type"`
	Received           bool           `json:"received"`
	Success            bool           `json:"success"`
	LatencyMS          float64        `json:"latency_ms"`
	Rcode              string         `json:"rcode"`
	Answers            []string       `json:"answers"`
	Error              string         `json:"error,omitempty"`
	Raw                string         `json:"raw,omitempty"`
	Pollution          string         `json:"pollution"`
	Reason             string         `json:"reason"`
	References         []Reference    `json:"references"`
	Override           string         `json:"override,omitempty"`
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
type CurrentEvaluation struct {
	Metrics
	WindowMinutes      int     `json:"window_minutes"`
	MinSamples         int     `json:"min_samples"`
	MinCoverageMinutes int     `json:"min_coverage_minutes"`
	CoveredMinutes     float64 `json:"covered_minutes"`
	QualityAt          int64   `json:"quality_at"`
	PendingReason      string  `json:"pending_reason"`
}
type ServerSummary struct {
	StatusHistory []StatusBucket `json:"status_history"`
	Server
	Current     CurrentEvaluation `json:"current"`
	Metrics     Metrics           `json:"metrics"`
	LastProbe   int64             `json:"last_probe"`
	NextDue     int64             `json:"next_due"`
	LastSuccess bool              `json:"last_success"`
	Failures    int               `json:"failures"`
}

// StatusBucket summarizes immutable evaluations captured at formal probe completion.
// Colors represent the worst observed status, never a duration or interpolated uptime.
type StatusBucket struct {
	Timestamp     int64               `json:"timestamp"`
	End           int64               `json:"end"`
	Snapshots     int64               `json:"snapshots"`
	Pollution     string              `json:"pollution"`
	Grade         string              `json:"grade"`
	QualityCounts map[string]int64    `json:"quality_counts,omitempty"`
	GradeCounts   map[string]int64    `json:"grade_counts,omitempty"`
	Latest        *EvaluationSnapshot `json:"latest,omitempty"`
}
type EvaluationSnapshot struct {
	Timestamp int64             `json:"timestamp"`
	Trusted   bool              `json:"trusted"`
	Current   CurrentEvaluation `json:"current"`
}

type HistoryPoint struct {
	Timestamp int64 `json:"timestamp"`
	Metrics
}
type RuntimeStatus struct {
	Refresh   *RefreshStatus `json:"refresh,omitempty"`
	Active    int            `json:"active"`
	Paused    bool           `json:"paused"`
	LastError string         `json:"last_error"`
	StartedAt int64          `json:"started_at"`
}

const Retention = 30 * 24 * time.Hour

type RefreshStatus struct {
	ID        int64  `json:"id"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Pending   bool   `json:"pending"`
	Error     string `json:"error,omitempty"`
}
type ServerImportItem struct {
	Row    int    `json:"row"`
	Key    string `json:"key"`
	Action string `json:"action"`
	Server Server `json:"server"`
}
type ServerImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}
