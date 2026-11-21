package api

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/VolkovDani/project-creator/internal/storage"
)

// A migration history later in search_path must not replace the history of the
// selected schema. Both schemas are private to this test; public is untouched.
func TestMigrationUsesSelectedSchemaHistory(t *testing.T) {
	dsn := testDatabase(t)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	selected := u.Query().Get("search_path")
	fallback := selected + "_fallback"
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err = admin.Exec(`CREATE SCHEMA ` + fallback + `; CREATE TABLE ` + fallback + `.schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO ` + fallback + `.schema_migrations VALUES(999)`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + fallback + ` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	q := u.Query()
	q.Set("search_path", selected+","+fallback)
	u.RawQuery = q.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatalf("fallback history leaked into selected schema: %v", err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(`SELECT max(version) FROM ` + selected + `.schema_migrations`).Scan(&version); err != nil || version != 17 {
		t.Fatalf("selected history: %d %v", version, err)
	}
	if err = db.QueryRow(`SELECT max(version) FROM ` + fallback + `.schema_migrations`).Scan(&version); err != nil || version != 999 {
		t.Fatalf("fallback modified: %d %v", version, err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name IN ('projects','requirements')`, selected).Scan(&count); err != nil || count != 2 {
		t.Fatalf("selected tables: %d %v", count, err)
	}
}
