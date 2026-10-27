package api

import (
	"context"
	"database/sql"
	"github.com/VolkovDani/project-creator/internal/storage"
	"os"
	"testing"
)

func TestOutboxIsAtomicAndDurable(t *testing.T) {
	dsn := testDatabase(t)
	db, err := storage.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("committed project event count=%d error=%v", n, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO requirements VALUES('r','a','p','draft',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("uncommitted event visible: %d %v", n, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = storage.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("restart/rollback event count=%d %v", n, err)
	}
	if _, err = db.Exec(`INSERT INTO requirements VALUES('r','a','p','text',1,'now','now'); UPDATE requirements SET text='new',version=2 WHERE id='r'; DELETE FROM requirements WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys @> ARRAY['project:p:requirements']`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("requirement event count=%d %v", n, err)
	}
}

func TestOutboxUpgradePreservesExistingProject(t *testing.T) {
	dsn := testDatabase(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	initial, err := os.ReadFile("../storage/migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(initial)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY);INSERT INTO schema_migrations VALUES(1);INSERT INTO projects VALUES('old','a','P','G','','before','before')`); err != nil {
		t.Fatal(err)
	}
	if err = storage.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE projects SET name='new' WHERE id='old'`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("upgraded event count: %d %v", n, err)
	}
	var created string
	if err = db.QueryRow(`SELECT created_at FROM projects WHERE id='old'`).Scan(&created); err != nil || created != "before" {
		t.Fatalf("existing project lost: %s %v", created, err)
	}
}
