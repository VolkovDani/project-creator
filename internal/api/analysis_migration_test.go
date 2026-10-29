package api

import (
	"context"
	"github.com/VolkovDani/project-creator/internal/storage"
	"testing"
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
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Simulate a version 4 database containing a previously completed attempt.
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO analysis_runs(id,workspace,project_id,status,outcome,generator_profile,embedding_profile,source_signature,sources,lease_until) VALUES('legacy','a','p','completed','insufficient','g','e','sig','[]',clock_timestamp()); ALTER TABLE analysis_runs DROP COLUMN reason_code; ALTER TABLE analysis_runs DROP COLUMN missing_requirement_ids; DELETE FROM schema_migrations WHERE version=5`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = storage.Migrate(context.Background(), s.db); err != nil {
			t.Fatal(err)
		}
	}
	var reason, missing string
	if err = s.db.QueryRow(`SELECT reason_code,missing_requirement_ids::text FROM analysis_runs WHERE id='legacy'`).Scan(&reason, &missing); err != nil || reason != "" || missing != "[]" {
		t.Fatalf("legacy diagnostics: %q %q %v", reason, missing, err)
	}
}
