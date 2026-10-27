package api

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func TestMissingVectorInstallPermission(t *testing.T) {
	// Use the same disposable server, but a new database without vector.
	isolated := testDatabase(t)
	u, err := url.Parse(isolated)
	if err != nil {
		t.Fatal(err)
	}
	name := u.Query().Get("search_path") + "_vector"
	admin, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var super bool
	if err = admin.QueryRow(`SELECT rolsuper FROM pg_roles WHERE rolname=current_user`).Scan(&super); err != nil {
		t.Fatal(err)
	}
	if !super {
		t.Skip("extension permission test requires disposable superuser test server")
	}
	if _, err = admin.Exec(`CREATE ROLE ` + name); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(`DROP ROLE ` + name); e != nil {
			t.Error(e)
		}
	}()
	if _, err = admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(`DROP DATABASE ` + name); e != nil {
			t.Error(e)
		}
	}()
	u.Path = "/" + name
	u.RawQuery = "sslmode=disable"
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`GRANT USAGE, CREATE ON SCHEMA public TO ` + name + `; SET ROLE ` + name); err != nil {
		t.Fatal(err)
	}
	err = storage.Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "pgvector unavailable") {
		t.Fatalf("expected actionable pgvector error: %v", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='schema_migrations'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed install left metadata: %d %v", count, err)
	}
}
