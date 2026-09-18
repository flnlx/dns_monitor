package store

import "fmt"

// Version 2 separates immutable detection evidence from the current comparison policy.
// The legacy numeric codes were unknown=0, clean=1, polluted=2. New codes are ordered
// by severity so SQL MAX and chart aggregation retain the worst observed verdict.
func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return fmt.Errorf("database schema %d is newer than this application", version)
	}
	if version == 0 {
		for _, query := range []string{
			"ALTER TABLE results ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 0",
			"ALTER TABLE servers ADD COLUMN trust_epoch INTEGER NOT NULL DEFAULT 0",
			`UPDATE results SET detected_pollution=CASE detected_pollution WHEN 1 THEN 2 WHEN 2 THEN 4 ELSE 0 END`,
			`UPDATE results SET effective_pollution=CASE
			 WHEN EXISTS(SELECT 1 FROM overrides o WHERE o.server_id=results.server_id AND o.domain=results.domain AND o.type=results.type AND o.verdict='polluted') THEN 4
			 WHEN EXISTS(SELECT 1 FROM overrides o WHERE o.server_id=results.server_id AND o.domain=results.domain AND o.type=results.type AND o.verdict='clean') THEN 2
			 WHEN detected_pollution=4 THEN 0 ELSE detected_pollution END`,
			`UPDATE rounds SET pollution=COALESCE((SELECT MAX(effective_pollution) FROM results WHERE round_id=rounds.id),0)`,
		} {
			if _, err = tx.Exec(query); err != nil {
				return fmt.Errorf("migrate comparison policy: %w", err)
			}
		}
	}
	_, err = tx.Exec(`
CREATE TABLE IF NOT EXISTS trusted_observations (
 server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 address TEXT NOT NULL, domain TEXT NOT NULL, type TEXT NOT NULL, value TEXT NOT NULL,
 ttl_seconds INTEGER NOT NULL, observed_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(server_id,domain,type,value)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS trusted_observations_subject ON trusted_observations(domain,type);
CREATE INDEX IF NOT EXISTS trusted_observations_age ON trusted_observations(observed_at);
CREATE TRIGGER IF NOT EXISTS invalidate_trusted_observations
AFTER UPDATE OF trusted,enabled,address ON servers
WHEN OLD.trusted<>NEW.trusted OR OLD.enabled<>NEW.enabled OR OLD.address<>NEW.address
BEGIN
 DELETE FROM trusted_observations WHERE server_id=NEW.id;
 UPDATE servers SET trust_epoch=trust_epoch+1 WHERE id=NEW.id;
END;
PRAGMA user_version=2;
`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
