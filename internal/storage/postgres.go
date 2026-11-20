// Package storage owns PostgreSQL connections and transactional schema migrations.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(dsn string) (*sql.DB, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || u.Path == "" {
		return nil, errors.New("PROJECT_CREATOR_DATABASE_URL must be an explicit PostgreSQL URL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("PostgreSQL connection failed; check database URL and server availability")
	}
	if err = Migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cannot begin PostgreSQL migration")
	}
	defer tx.Rollback()
	// Scope migration metadata to the selected schema, never a fallback schema.
	if _, err = tx.ExecContext(ctx, `SELECT set_config('search_path',quote_ident(current_schema()),true)`); err != nil {
		return err
	}
	// Serialize startup across instances, including the first schema creation.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183091)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}
	if version > 16 {
		return errors.New("database schema version is newer than supported")
	}
	// Install only if absent: runtime users can use an extension installed by an administrator.
	var vector bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')`).Scan(&vector); err != nil {
		return err
	}
	if !vector {
		if _, err = tx.ExecContext(ctx, `CREATE EXTENSION vector WITH SCHEMA public`); err != nil {
			return errors.New("pgvector unavailable: install the vector extension in this database as an administrator, then retry")
		}
	}
	var vectorType bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_type t JOIN pg_extension e ON e.extnamespace=t.typnamespace WHERE e.extname='vector' AND t.typname='vector')`).Scan(&vectorType); err != nil {
		return err
	}
	if !vectorType {
		return errors.New("pgvector extension has no vector type")
	}
	for i, name := range []string{"001_initial.sql", "002_cache_events.sql", "003_rag.sql", "004_analysis.sql", "005_analysis_diagnostics.sql", "006_progressive_analysis.sql", "007_full_analysis.sql", "008_index_queue.sql", "009_analysis_live.sql", "010_analysis_replacement.sql", "011_laya_pipeline.sql", "012_requirement_summaries.sql", "013_laya_optional.sql", "014_glossary.sql", "015_classification.sql", "016_classification_queue.sql"} {
		next := i + 1
		if version >= next {
			continue
		}
		script, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf("PostgreSQL schema migration failed: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, next); err != nil {
			return err
		}
	}

	return tx.Commit()
}
