package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/VolkovDani/project-creator/internal/storage"
)

// Each test uses a fresh schema; reopening the same URL tests persistence.
func testDatabase(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required in CI")
		}
		t.Skip("PostgreSQL integration: set TEST_DATABASE_URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	var b [12]byte
	if _, err = rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(b[:])
	if _, err = db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer db.Close()
		if _, err := db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestPostgresMigrationAndRollback(t *testing.T) {
	dsn := testDatabase(t)
	db, err := storage.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO projects VALUES('rollback','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO requirements VALUES('bad','other','p','text',1,'now','now')`); err == nil {
		t.Fatal("cross-workspace FK accepted")
	}
	if err = tx.Commit(); err == nil {
		t.Fatal("failed transaction committed")
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM projects WHERE id='rollback'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollback count=%d err=%v", n, err)
	}
}

func TestPostgresConcurrentOptimisticUpdate(t *testing.T) {
	db, err := storage.Open(testDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','text',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := db.Exec(`UPDATE requirements SET version=version+1 WHERE id='r' AND version=1`)
			if e != nil {
				t.Error(e)
				return
			}
			n, e := r.RowsAffected()
			if e != nil {
				t.Error(e)
				return
			}
			results <- n
		}()
	}
	wg.Wait()
	close(results)
	var total int64
	for n := range results {
		total += n
	}
	if total != 1 {
		t.Fatalf("successful updates=%d", total)
	}
}

func TestMigrationFailureRollsBackDDL(t *testing.T) {
	dsn := testDatabase(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE requirements(broken INTEGER)`); err != nil {
		t.Fatal(err)
	}
	err = storage.Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "migration failed") {
		t.Fatalf("expected migration error: %v", err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('projects','schema_migrations')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("partially applied migration: %d %v", n, err)
	}
}

func TestConcurrentStartup(t *testing.T) {
	dsn := testDatabase(t)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := storage.Open(dsn)
			if err != nil {
				t.Error(err)
				return
			}
			db.Close()
		}()
	}
	wg.Wait()
	db, err := storage.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != 19 {
		t.Fatalf("migration count %d: %v", n, err)
	}
}
