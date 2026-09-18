package store

import (
	"database/sql"
	"errors"
	"math"
	"net/netip"
	"strings"
	"time"

	"dnsmonitor/internal/model"
)

const maxTrustedSubjectAddresses = 256
const maxTrustedAddresses = 16384

// SaveTrustedObservation records only a real successful IPv4 lookup. The source
// epoch prevents an old in-flight query from returning after trust was revoked.
func (s *Store) SaveTrustedObservation(server model.Server, result model.ProbeResult) error {
	domain, typ := normalizeSubject(result.Domain, result.Type)
	if !result.Success || typ != "A" || domain == "" || len(result.Records) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := scanServer(tx.QueryRow("SELECT "+serverColumns+" FROM servers WHERE id=?", server.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !current.Enabled || !current.Trusted || current.Address != server.Address || current.TrustEpoch != server.TrustEpoch {
		return nil
	}
	for _, record := range result.Records {
		ip, err := netip.ParseAddr(strings.TrimSpace(record.Value))
		if err != nil || !ip.Is4() || record.ObservedAt <= 0 || record.TTLSeconds < 0 {
			continue
		}
		// Retention also bounds extreme upstream TTL values. TTL=0 remains history only.
		expires := min(record.ExpiresAt, record.ObservedAt+int64(model.Retention/time.Millisecond))
		if expires < record.ObservedAt {
			expires = record.ObservedAt
		}
		_, err = tx.Exec(`INSERT INTO trusted_observations(server_id,address,domain,type,value,ttl_seconds,observed_at,expires_at)
VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(server_id,domain,type,value) DO UPDATE SET
address=excluded.address,ttl_seconds=excluded.ttl_seconds,observed_at=excluded.observed_at,expires_at=excluded.expires_at
WHERE excluded.observed_at>=trusted_observations.observed_at`, current.ID, current.Address, domain, typ, ip.String(), record.TTLSeconds, record.ObservedAt, expires)
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM trusted_observations WHERE server_id=? AND domain=? AND type=? AND value IN
(SELECT value FROM trusted_observations WHERE server_id=? AND domain=? AND type=? ORDER BY observed_at DESC,value LIMIT -1 OFFSET ?)`, current.ID, domain, typ, current.ID, domain, typ, maxTrustedSubjectAddresses); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM trusted_observations WHERE (server_id,domain,type,value) IN
(SELECT server_id,domain,type,value FROM trusted_observations ORDER BY observed_at DESC,server_id,domain,type,value LIMIT -1 OFFSET ?)`, maxTrustedAddresses); err != nil {
		return err
	}
	return tx.Commit()
}

func referenceCutoff(now int64, historyHours float64) int64 {
	if math.IsNaN(historyHours) || historyHours <= 0 {
		return now
	}
	if math.IsInf(historyHours, 0) || historyHours > 720 {
		historyHours = 720
	}
	return now - int64(historyHours*float64(time.Hour/time.Millisecond))
}

// TrustedReferences returns the union of unexpired TTL records and observations
// inside the configured history window. It never refreshes observed_at on reads.
func (s *Store) TrustedReferences(domain model.Domain, now int64, historyHours float64) ([]model.Reference, error) {
	name, typ := normalizeSubject(domain.Name, domain.Type)
	cutoff := referenceCutoff(now, historyHours)
	rows, err := s.db.Query(`SELECT o.server_id,o.address,o.value,o.ttl_seconds,o.observed_at,o.expires_at
FROM trusted_observations o JOIN servers s ON s.id=o.server_id
WHERE o.domain=? AND o.type=? AND s.trusted=1 AND s.enabled=1 AND o.address=s.address
AND o.observed_at>=? AND (o.expires_at>? OR (? AND o.observed_at>?))
ORDER BY o.server_id,o.value`, name, typ, now-int64(model.Retention/time.Millisecond), now, historyHours > 0, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := make([]model.Reference, 0)
	for rows.Next() {
		var id int64
		var address string
		var record model.AnswerRecord
		if err = rows.Scan(&id, &address, &record.Value, &record.TTLSeconds, &record.ObservedAt, &record.ExpiresAt); err != nil {
			return nil, err
		}
		if len(refs) == 0 || refs[len(refs)-1].ServerID != id {
			refs = append(refs, model.Reference{ServerID: id, Address: address, Rcode: "NOERROR", Success: true, Answers: []string{}, Records: []model.AnswerRecord{}})
		}
		ref := &refs[len(refs)-1]
		ref.Timestamp = max(ref.Timestamp, record.ObservedAt)
		ref.Answers = append(ref.Answers, record.Value)
		ref.Records = append(ref.Records, record)
	}
	return refs, rows.Err()
}

// Pruning is bounded by the fixed global pool limit, not by raw probe volume.
func (s *Store) PruneTrustedReferences(now int64, historyHours float64) error {
	_, err := s.db.Exec(`DELETE FROM trusted_observations WHERE observed_at<? OR
(expires_at<=? AND (? OR observed_at<=?))`, now-int64(model.Retention/time.Millisecond), now, historyHours <= 0, referenceCutoff(now, historyHours))
	return err
}
