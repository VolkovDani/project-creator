package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/storage"
)

func TestAnalysisRunStoragePrepared(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM analysis_runs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("new analysis history contains %d runs", count)
	}
}

func TestAnalysisDiagnosticsUpgradeExistingHistory(t *testing.T) {
	db := analysisDatabaseAtVersion(t, 4)
	_, err := db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO analysis_runs(id,workspace,project_id,status,outcome,generator_profile,embedding_profile,source_signature,sources,lease_until) VALUES('legacy','a','p','completed','insufficient','g','e','sig','[]',clock_timestamp())`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = storage.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	var reason, missing string
	if err = db.QueryRow(`SELECT reason_code,missing_requirement_ids::text FROM analysis_runs WHERE id='legacy'`).Scan(&reason, &missing); err != nil || reason != "" || missing != "[]" {
		t.Fatalf("legacy diagnostics: %q %q %v", reason, missing, err)
	}
}

func analysisDatabaseAtVersion(t *testing.T, version int) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", testDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public; CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	files := []string{"001_initial.sql", "002_cache_events.sql", "003_rag.sql", "004_analysis.sql", "005_analysis_diagnostics.sql"}
	for i, name := range files[:version] {
		script, err := os.ReadFile("../storage/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(script)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO schema_migrations VALUES($1)`, i+1); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestProgressMigrationPreservesSnapshotsAndInterruptsLegacyRunning(t *testing.T) {
	db := analysisDatabaseAtVersion(t, 5)
	if _, err := db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	sources := `[{"id":"left","version":1,"text":"Original left"},{"id":"right","version":2,"text":"Original right"}]`
	findings := `[{"leftId":"left","leftVersion":1,"rightId":"right","rightVersion":2,"verdict":"collision","explanation":"Synthetic finding"}]`
	for _, status := range []string{"completed", "failed", "running"} {
		if _, err := db.Exec(`INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,findings,lease_until) VALUES($1,'a','p',$1,'g','e','snapshot',$2::jsonb,$3::jsonb,clock_timestamp())`, status, sources, findings); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := storage.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"completed", "failed", "running"} {
		var status, stage, code string
		var total sql.NullInt64
		var processed int
		var raw []byte
		if err := db.QueryRow(`SELECT status,stage,error_code,total_pairs,processed_pairs,pairs FROM analysis_runs WHERE id=$1`, id).Scan(&status, &stage, &code, &total, &processed, &raw); err != nil {
			t.Fatal(err)
		}
		var pairs []analysis.PairResult
		if err := json.Unmarshal(raw, &pairs); err != nil {
			t.Fatal(err)
		}
		if len(pairs) != 1 || pairs[0].Status != "completed" || pairs[0].Left.Text != "Original left" || pairs[0].Right.Version != 2 || processed != 1 || stage != "finished" {
			t.Fatalf("legacy result changed: id=%s pairs=%+v processed=%d stage=%s", id, pairs, processed, stage)
		}
		if id == "completed" && (!total.Valid || total.Int64 != 1) {
			t.Fatalf("completed progress missing: %+v", total)
		}
		if id != "completed" && total.Valid {
			t.Fatalf("unknown legacy candidate count was invented: %+v", total)
		}
		if id == "running" && (status != "failed" || code != "INTERRUPTED") {
			t.Fatalf("legacy running attempt hangs: %s %s", status, code)
		}
	}
}
