package legacy

import (
	"context"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func source(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(1),(2); CREATE TABLE projects(id TEXT PRIMARY KEY,workspace TEXT,name TEXT,goal TEXT,description TEXT,created_at TEXT,updated_at TEXT); CREATE TABLE requirements(id TEXT PRIMARY KEY,workspace TEXT,project_id TEXT,text TEXT,version INTEGER,created_at TEXT,updated_at TEXT); INSERT INTO projects VALUES('p','a','Проект','goal','','2026-01-01T00:00:00.123Z','2026-01-01T00:00:00.123Z'); INSERT INTO requirements VALUES('r','a','p','Текст',7,'before','after');`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestValidateSource(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"valid", ""},
		{"future", "INSERT INTO schema_migrations VALUES(3)"},
		{"old", "DELETE FROM schema_migrations WHERE version=2"},
		{"orphan", "UPDATE requirements SET project_id='missing'"},
		{"workspace", "UPDATE requirements SET workspace='other'"},
		{"version", "UPDATE requirements SET version=0"},
		{"overflow", "UPDATE requirements SET version=2147483648"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := source(t)
			if tc.sql != "" {
				if _, err := db.Exec(tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err = validateSource(context.Background(), tx)
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

func TestBackupReadOnlySource(t *testing.T) {
	db := source(t)
	var path string
	var seq int
	var name string
	if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db.Close()
	uri := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
	ro, err := sql.Open("sqlite3", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err = Backup(context.Background(), ro, backup); err != nil {
		t.Fatal(err)
	}
	if err = Backup(context.Background(), ro, backup); err == nil {
		t.Fatal("overwrote backup")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
	b, err := sql.Open("sqlite3", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var text string
	if err = b.QueryRow("SELECT text FROM requirements").Scan(&text); err != nil || text != "Текст" {
		t.Fatalf("backup: %q %v", text, err)
	}
}
