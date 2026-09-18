package store

import (
	"errors"

	"dnsmonitor/internal/model"
)

// CurrentEvaluation evaluates recent performance and the latest formal round's
// answer quality independently of the user-selected historical chart range.
func (s *Store) CurrentEvaluation(serverID, now int64) (model.CurrentEvaluation, error) {
	server, err := s.GetServer(serverID)
	if err != nil {
		return model.CurrentEvaluation{}, err
	}
	config, err := s.GetConfig()
	if err != nil {
		return model.CurrentEvaluation{}, err
	}
	return s.currentEvaluation(server, config, now)
}

func (s *Store) currentEvaluation(server model.Server, config model.Config, now int64) (model.CurrentEvaluation, error) {
	value := model.CurrentEvaluation{
		WindowMinutes:      config.RatingWindowMinutes,
		MinSamples:         config.RatingMinSamples,
		MinCoverageMinutes: config.RatingMinCoverageMinutes,
	}
	if now <= 0 || value.WindowMinutes <= 0 || value.MinSamples <= 0 || value.MinCoverageMinutes < 0 || value.MinCoverageMinutes > value.WindowMinutes {
		return value, errors.New("invalid current evaluation time or thresholds")
	}
	window := int64(value.WindowMinutes) * 60 * 1000
	since := now - window
	a := accumulator{buckets: map[int64]int64{}}
	var received int64
	// Include only one predecessor for an interval crossing the window boundary.
	// Both arms use rounds_server_time; no raw JSON or whole-retention scan is needed.
	query := `WITH eligible AS (
 SELECT finished_at,covered_until,samples,received,successes,latency_count,latency_sum
 FROM rounds WHERE server_id=? AND auxiliary=0 AND finished_at>=? AND finished_at<=?
 UNION ALL
 SELECT finished_at,covered_until,samples,received,successes,latency_count,latency_sum
 FROM rounds WHERE id=(SELECT id FROM rounds WHERE server_id=? AND auxiliary=0 AND finished_at<? ORDER BY finished_at DESC,id DESC LIMIT 1) AND covered_until>?
 ) SELECT
 COALESCE(SUM(CASE WHEN finished_at>=? THEN samples ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN finished_at>=? THEN received ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN finished_at>=? THEN successes ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN finished_at>=? THEN latency_count ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN finished_at>=? THEN latency_sum ELSE 0 END),0),
 COALESCE(SUM(MAX(0,MIN(covered_until,?)-MAX(finished_at,?))),0),
 COALESCE(SUM(MAX(0,MIN(covered_until,?)-MAX(finished_at,?))*1.0*received/MAX(1,samples)),0),
 COALESCE(MAX(CASE WHEN finished_at>=? THEN finished_at ELSE 0 END),0)
 FROM eligible`
	err := s.db.QueryRow(query, server.ID, since, now, server.ID, since, since,
		since, since, since, since, since, now, since, now, since, since).
		Scan(&a.m.Samples, &received, &a.successes, &a.latencyCount, &a.latencySum, &a.covered, &a.available, &value.QualityAt)
	if err != nil {
		return value, err
	}
	rows, err := s.db.Query(`SELECT b.bucket_ms,SUM(b.count) FROM rounds r JOIN latency_buckets b ON b.round_id=r.id
 WHERE r.server_id=? AND r.auxiliary=0 AND r.finished_at>=? AND r.finished_at<=? GROUP BY b.bucket_ms`, server.ID, since, now)
	if err != nil {
		return value, err
	}
	for rows.Next() {
		var bucket, count int64
		if err = rows.Scan(&bucket, &count); err != nil {
			rows.Close()
			return value, err
		}
		a.buckets[bucket] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return value, err
	}

	if server.Trusted {
		a.pollution = 1
	} else if value.QualityAt > 0 {
		var severity int
		// Unknown A answers outrank matched/clean answers in the latest round.
		// Other types are not automatically comparable, but an explicit manual
		// verdict (effective_pollution > 0) still participates.
		err = s.db.QueryRow(`SELECT COALESCE(MAX(CASE effective_pollution
 WHEN 4 THEN 5 WHEN 3 THEN 4 WHEN 0 THEN 3 WHEN 2 THEN 2 WHEN 1 THEN 1 ELSE 3 END),0)
 FROM results WHERE round_id=(SELECT id FROM rounds WHERE server_id=? AND auxiliary=0 AND finished_at>=? AND finished_at<=? ORDER BY finished_at DESC,id DESC LIMIT 1)
 AND (type='A' OR effective_pollution>0)`, server.ID, since, now).Scan(&severity)
		if err != nil {
			return value, err
		}
		switch severity {
		case 5:
			a.pollution = 4
		case 4:
			a.pollution = 3
		case 2:
			a.pollution = 2
		case 1:
			a.pollution = 1
		}
	}
	// With an explicit zero coverage threshold, the first just-finished probe
	// has no elapsed interval yet. Use its received ratio instead of scoring a
	// successful query as unavailable. Once coverage exists, time weighting wins.
	if a.covered == 0 && a.m.Samples > 0 {
		a.m.Availability = 100 * float64(received) / float64(a.m.Samples)
	}
	value.CoveredMinutes = a.covered / 60000
	value.Metrics = a.finishWithThresholds(window, int64(value.MinSamples), int64(value.MinCoverageMinutes)*60000)
	switch {
	case a.pollution >= 3:
		// A current suspicious/polluted answer is actionable even before warm-up.
	case value.Samples == 0:
		value.Grade = "pending"
		value.PendingReason = "no_samples"
	case !server.Trusted && a.pollution == 0:
		value.Grade = "pending"
		value.PendingReason = "quality_unknown"
	case value.Samples < int64(value.MinSamples):
		value.PendingReason = "insufficient_samples"
	case a.covered < float64(value.MinCoverageMinutes)*60000:
		value.PendingReason = "insufficient_coverage"
	}
	return value, nil
}
