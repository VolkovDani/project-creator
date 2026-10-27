package api

import (
	"github.com/VolkovDani/project-creator/internal/storage"
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
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys @> ARRAY['project:p:requirements']`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("requirement event count=%d %v", n, err)
	}
}
