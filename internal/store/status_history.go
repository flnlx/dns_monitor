package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"dnsmonitor/internal/model"
)

// Capture within the round transaction: neither a partial probe nor an evaluation
// using a later round can leak into history. Later overrides/config changes do not
// rewrite these snapshots. Auxiliary reference lookups never enter rating history.
func saveEvaluationSnapshot(tx *sql.Tx, serverID, roundID, at int64) error {
	server, err := scanServer(tx.QueryRow("SELECT "+serverColumns+" FROM servers WHERE id=?", serverID))
	if err != nil {
		return err
	}
	var raw string
	if err = tx.QueryRow("SELECT value FROM config WHERE id=1").Scan(&raw); err != nil {
		return err
	}
	config := model.DefaultConfig()
	if err = json.Unmarshal([]byte(raw), &config); err != nil {
		return err
	}
	evaluation, err := evaluateCurrent(tx, server, config, at)
	if err != nil {
		return err
	}
	// Trust exempts comparison; it is not evidence of a fresh reference match.
	if server.Trusted {
		evaluation.Pollution = "clean"
	}
	encoded, err := json.Marshal(evaluation)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO evaluation_snapshots(round_id,server_id,timestamp,pollution,grade,trusted,evaluation) VALUES(?,?,?,?,?,?,?)`,
		roundID, serverID, at, evaluation.Pollution, evaluation.Grade, server.Trusted, string(encoded))
	return err
}

var qualitySeverity = map[string]int{"matched": 1, "clean": 2, "unknown": 3, "suspicious": 4, "polluted": 5}
var gradeSeverity = map[string]int{"A": 1, "B": 2, "C": 3, "D": 4, "pending": 5, "E": 6, "F": 7, "unavailable": 8}

// StatusHistory uses fixed-size observation buckets (30m / 3h / 12h for the UI
// ranges). Empty buckets stay empty, including all pre-upgrade history. We do not
// infer past grades using today's configuration or propagate samples across gaps.
func (s *Store) StatusHistory(serverID, since, now int64, includeLatest bool) ([]model.StatusBucket, error) {
	if now <= since || now-since > int64(model.Retention/time.Millisecond) {
		return nil, errors.New("invalid status history range")
	}
	count := int64(48)
	if now-since > int64(24*time.Hour/time.Millisecond) {
		count = 56
	}
	if now-since > int64(7*24*time.Hour/time.Millisecond) {
		count = 60
	}
	step := (now - since + count - 1) / count
	count = (now - since + step - 1) / step
	points := make([]model.StatusBucket, count)
	for i := range points {
		points[i] = model.StatusBucket{Timestamp: since + int64(i)*step, End: min(now, since+int64(i+1)*step), Pollution: "no_data", Grade: "no_data"}
	}
	// Aggregate indexed statuses; decode at most one evaluation per detail bucket.
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT MIN(?,(timestamp-?)/?) AS bucket,pollution,grade,COUNT(*)
 FROM evaluation_snapshots WHERE server_id=? AND timestamp>=? AND timestamp<=?
 GROUP BY bucket,pollution,grade`, count-1, since, step, serverID, since, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var index, n int64
		var quality, grade string
		if err = rows.Scan(&index, &quality, &grade, &n); err != nil {
			return nil, err
		}
		point := &points[index]
		if point.QualityCounts == nil {
			point.QualityCounts = map[string]int64{}
			point.GradeCounts = map[string]int64{}
		}
		point.Snapshots += n
		point.QualityCounts[quality] += n
		point.GradeCounts[grade] += n
		if qualitySeverity[quality] > qualitySeverity[point.Pollution] {
			point.Pollution = quality
		}
		if gradeSeverity[grade] > gradeSeverity[point.Grade] {
			point.Grade = grade
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if includeLatest {
		for i := range points {
			point := &points[i]
			if point.Snapshots == 0 {
				continue
			}
			latest := &model.EvaluationSnapshot{}
			var raw string
			// Interior buckets are [start,end); the last includes exactly now.
			end := point.End - 1
			if i == len(points)-1 {
				end = now
			}
			err = tx.QueryRow(`SELECT timestamp,trusted,evaluation FROM evaluation_snapshots
 WHERE server_id=? AND timestamp>=? AND timestamp<=? ORDER BY timestamp DESC,round_id DESC LIMIT 1`,
				serverID, point.Timestamp, end).Scan(&latest.Timestamp, &latest.Trusted, &raw)
			if err != nil {
				return nil, err
			}
			if err = json.Unmarshal([]byte(raw), &latest.Current); err != nil {
				return nil, err
			}
			point.Latest = latest
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return points, nil
}
