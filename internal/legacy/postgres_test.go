package legacy

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func target(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("legacy_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPostgresImportPreservesDataAndRefusesRepeat(t *testing.T) {
	src, dst := source(t), target(t)
	counts, err := Import(context.Background(), src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (Counts{Projects: 1, Requirements: 1}) {
		t.Fatal(counts)
	}
	var text, created, updated string
	var version int
	if err = dst.QueryRow("SELECT text,version,created_at,updated_at FROM requirements WHERE id='r'").Scan(&text, &version, &created, &updated); err != nil {
		t.Fatal(err)
	}
	if text != "Текст" || version != 7 || created != "before" || updated != "after" {
		t.Fatalf("data changed: %q %d %q %q", text, version, created, updated)
	}
	if _, err = Import(context.Background(), src, dst); err == nil {
		t.Fatal("accepted repeated import")
	}
	var n int
	if err = src.QueryRow("SELECT count(*) FROM requirements").Scan(&n); err != nil || n != 1 {
		t.Fatalf("source changed: %d %v", n, err)
	}
}

func TestPostgresImportRollsBackOnDestinationFailure(t *testing.T) {
	src, dst := source(t), target(t)
	if _, err := dst.Exec("ALTER TABLE requirements ADD CONSTRAINT reject_legacy CHECK (text != 'Текст')"); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(context.Background(), src, dst); err == nil {
		t.Fatal("accepted invalid row")
	}
	var n int
	if err := dst.QueryRow("SELECT (SELECT count(*) FROM projects)+(SELECT count(*) FROM requirements)").Scan(&n); err != nil || n != 0 {
		t.Fatalf("partial import: %d %v", n, err)
	}
}
