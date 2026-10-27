// Package legacy provides an explicit one-time SQLite import. It never deletes
// or changes the source database. Stop writers before backing up and importing.
package legacy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

type Counts struct{ Projects, Requirements int64 }

// Backup creates a consistent SQLite backup with restrictive permissions. The
// destination must not exist; a failed backup removes only the newly created file.
func Backup(ctx context.Context, src *sql.DB, path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	if err = f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	if _, err = src.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		os.Remove(path)
		return fmt.Errorf("SQLite backup: %w", err)
	}
	// SQLite VACUUM INTO does not itself fsync the output file.
	f, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}
	return closeErr
}

func validateSource(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if result != "ok" {
			rows.Close()
			return errors.New("SQLite integrity check failed")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var count, min, max int
	if err = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(min(version),0),coalesce(max(version),0) FROM schema_migrations").Scan(&count, &min, &max); err != nil {
		return err
	}
	if count != 2 || min != 1 || max != 2 {
		return errors.New("SQLite import requires schema version 2 (migration rows 1 and 2)")
	}
	var invalid int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM requirements r LEFT JOIN projects p ON p.id=r.project_id AND p.workspace=r.workspace WHERE p.id IS NULL OR r.version IS NULL OR r.version<1 OR r.version>2147483647 OR typeof(r.version)!='integer'`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New("SQLite contains orphan/cross-workspace requirements or invalid versions")
	}
	return nil
}

// Import preserves IDs, source strings and versions. PostgreSQL must already be
// migrated. Both target tables must be empty; every write is committed together.
func Import(ctx context.Context, src, dst *sql.DB) (Counts, error) {
	var result Counts
	read, err := src.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer read.Rollback()
	if err = validateSource(ctx, read); err != nil {
		return result, err
	}
	write, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer write.Rollback()
	if _, err = write.ExecContext(ctx, "LOCK TABLE projects, requirements IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return result, err
	}
	var existing int64
	if err = write.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM projects)+(SELECT count(*) FROM requirements)").Scan(&existing); err != nil {
		return result, err
	}
	if existing != 0 {
		return result, errors.New("PostgreSQL target must be empty")
	}
	projects, err := read.QueryContext(ctx, "SELECT id,workspace,name,goal,description,created_at,updated_at FROM projects ORDER BY id")
	if err != nil {
		return result, err
	}
	for projects.Next() {
		var id, ws, name, goal, description, created, updated string
		if err = projects.Scan(&id, &ws, &name, &goal, &description, &created, &updated); err != nil {
			projects.Close()
			return result, err
		}
		if id == "" || ws == "" {
			projects.Close()
			return result, errors.New("empty project ID/workspace")
		}
		if _, err = write.ExecContext(ctx, `INSERT INTO projects(id,workspace,name,goal,description,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, ws, name, goal, description, created, updated); err != nil {
			projects.Close()
			return result, err
		}
		result.Projects++
	}
	err = projects.Err()
	projects.Close()
	if err != nil {
		return result, err
	}
	requirements, err := read.QueryContext(ctx, "SELECT id,workspace,project_id,text,version,created_at,updated_at FROM requirements ORDER BY id")
	if err != nil {
		return result, err
	}
	for requirements.Next() {
		var id, ws, project, text, created, updated string
		var version int64
		if err = requirements.Scan(&id, &ws, &project, &text, &version, &created, &updated); err != nil {
			requirements.Close()
			return result, err
		}
		if id == "" {
			requirements.Close()
			return result, errors.New("empty requirement ID")
		}
		if _, err = write.ExecContext(ctx, `INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, ws, project, text, version, created, updated); err != nil {
			requirements.Close()
			return result, err
		}
		result.Requirements++
	}
	err = requirements.Err()
	requirements.Close()
	if err != nil {
		return result, err
	}
	var actual Counts
	if err = write.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM projects),(SELECT count(*) FROM requirements)").Scan(&actual.Projects, &actual.Requirements); err != nil {
		return result, err
	}
	if actual != result {
		return result, errors.New("import count verification failed")
	}
	if err = write.Commit(); err != nil {
		return result, err
	}
	return result, nil
}
