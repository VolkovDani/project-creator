package api

import (
	"context"
	"database/sql"
	"time"
)

// A restart can change the configured embedding profile without changing source
// rows. Durable invalidations keep already-open clients from retaining the old
// profile's status/results while resuming an otherwise valid SSE cursor.
func announceStartup(db *sql.DB, tokens map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183092)`); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, workspace := range tokens {
		if seen[workspace] {
			continue
		}
		seen[workspace] = true
		if _, err = tx.ExecContext(ctx, `INSERT INTO cache_events(id,workspace,keys) VALUES(md5(random()::text||clock_timestamp()::text),$1,ARRAY['projects'])`, workspace); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO cache_events(id,workspace,keys) SELECT md5(random()::text||clock_timestamp()::text||id),workspace,ARRAY['projects','project:'||id,'project:'||id||':requirements'] FROM projects WHERE workspace=$1 ORDER BY id`, workspace); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1)`); err != nil {
		return err
	}
	return tx.Commit()
}
