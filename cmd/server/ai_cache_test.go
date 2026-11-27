package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func TestClearAICacheRejectsInvalidSelectionBeforeDatabase(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--all", "--section", "classification-form"},
		{"--group", "classification", "--group", "diagrams"},
		{"--section", "unknown"},
		{"--group", "unknown"},
		{"--all", "--project-id", " ", "--yes"},
		{"--all", "unexpected"},
	} {
		var output bytes.Buffer
		err := runClearAICache(t.Context(), args, func(string) string {
			t.Fatal("invalid arguments reached database configuration")
			return ""
		}, &output)
		if err == nil || output.Len() != 0 {
			t.Fatalf("args %q: output %q, error %v", args, &output, err)
		}
	}
}

func TestClearAICacheKeepsPreviewAndDeletionScoped(t *testing.T) {
	const env = "TEST_DATABASE_URL"
	dsn := os.Getenv(env)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal(env + " required in CI")
		}
		t.Skip("set " + env + " to exercise PostgreSQL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := "ai_cache_" + strings.ToLower(rand.Text())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	scoped, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := scoped.Query()
	query.Set("search_path", schema)
	scoped.RawQuery = query.Encode()
	db, err := storage.Open(scoped.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO projects(id,workspace,name,goal,description,created_at,updated_at)
		VALUES('p','a','P','Goal','','now','now'),('q','b','Q','Goal','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		VALUES('req-p','a','p','First',1,'now','now'),('req-q','b','q','Second',1,'now','now');
		INSERT INTO classification_explanation_cache(workspace,project_id,requirement_id,axis,cache_key,created_at,result)
		VALUES('a','p','req-p','meaning',decode(repeat('a',64),'hex'),now(),'{}'),
		      ('a','p','req-p','form',decode(repeat('b',64),'hex'),now(),'{}'),
		      ('b','q','req-q','meaning',decode(repeat('c',64),'hex'),now(),'{}'),
		      ('b','q','req-q','form',decode(repeat('d',64),'hex'),now(),'{}');
		INSERT INTO process_graph_cache(workspace,project_id,profile,format_version,snapshot_key,reusable,term_id,keyword,result)
		VALUES('a','p','prof',1,'snapshot',true,'','','{}'),('b','q','prof',1,'snapshot',true,'','','{}')`)
	getenv := func(key string) string {
		if key != "PROJECT_CREATOR_DATABASE_URL" {
			t.Fatalf("unexpected environment lookup %q", key)
		}
		return scoped.String()
	}
	check := func(want string, args ...string) {
		t.Helper()
		var output bytes.Buffer
		err := runClearAICache(t.Context(), args, getenv, &output)
		if err != nil || output.String() != want+"\n" {
			t.Fatalf("args %q: output %q, error %v, want %s", args, &output, err, want)
		}
	}
	check("6", "--all")
	check("3", "--all", "--project-id", "p")
	check("2", "--section", "classification-form")

	// Failure in the process graph delete must roll back the classification delete.
	exec(`CREATE FUNCTION reject_graph_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'private trigger detail'; END; $$;
		CREATE TRIGGER reject_graph_delete BEFORE DELETE ON process_graph_cache
		FOR EACH STATEMENT EXECUTE FUNCTION reject_graph_delete()`)
	var output bytes.Buffer
	err = runClearAICache(t.Context(), []string{"--all", "--yes"}, getenv, &output)
	if err == nil || strings.Contains(err.Error(), "private trigger detail") || output.Len() != 0 {
		t.Fatalf("transaction failure leaked or was ignored: output %q, error %v", &output, err)
	}
	exec(`DROP TRIGGER reject_graph_delete ON process_graph_cache`)
	check("6", "--all")
	check("1", "--section", "classification-meaning", "--project-id", "p", "--yes")
	check("1", "--group", "diagrams", "--project-id", "p", "--yes")
	check("1", "--section", "classification-form", "--project-id", "p")
	check("3", "--all", "--project-id", "q")
	check("3", "--group", "classification", "--yes")
	check("1", "--section", "process-graphs", "--yes")
	check("0", "--all")
	var invalidations int
	if err := db.QueryRow(`SELECT count(*) FROM cache_events
		WHERE keys && ARRAY['project:p:process-graph','project:q:process-graph']`).Scan(&invalidations); err != nil || invalidations != 2 {
		t.Fatalf("process graph invalidations=%d, error %v", invalidations, err)
	}
}
