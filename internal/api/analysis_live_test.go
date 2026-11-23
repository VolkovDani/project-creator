package api

import (
	"context"
	"testing"
)

func TestAnalysisEventsCoalesceToAuthorizedLatestSnapshot(t *testing.T) {
	db := eventDB(t)
	var version int
	if err := db.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 19 {
		t.Fatalf("migration version = %d, want 19", version)
	}
	if _, err := db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	var cursorID string
	if err := db.QueryRow(`SELECT id FROM cache_events WHERE workspace='a' ORDER BY seq DESC LIMIT 1`).Scan(&cursorID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,lease_until) VALUES('r1','a','p','running','g','e','sig','[]','infinity')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE analysis_runs SET stage='finding_pairs' WHERE id='r1'; UPDATE analysis_runs SET stage='comparing' WHERE id='r1'`); err != nil {
		t.Fatal(err)
	}
	batch, err := readEventBatch(context.Background(), db, "a", eventCursor{id: cursorID})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.progress) != 1 {
		t.Fatalf("progress events = %d, want one: %+v", len(batch.progress), batch.progress)
	}
	progress := batch.progress[0]
	if progress.SchemaVersion != 1 || progress.ProjectID != "p" || progress.RunID != "r1" || progress.Snapshot.Stage != "comparing" || progress.Revision == "" || progress.CursorID != batch.cursor.id {
		t.Fatalf("progress event: %+v cursor=%+v", progress, batch.cursor)
	}
	foreign, err := readEventBatch(context.Background(), db, "b", eventCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(foreign.progress) != 0 {
		t.Fatalf("foreign workspace received analysis: %+v", foreign.progress)
	}
}

func TestAnalysisProgressPreservesJournalOrderAcrossProjects(t *testing.T) {
	db := eventDB(t)
	if _, err := db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','a','Q','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	var cursorID string
	if err := db.QueryRow(`SELECT id FROM cache_events WHERE workspace='a' ORDER BY seq DESC LIMIT 1`).Scan(&cursorID); err != nil {
		t.Fatal(err)
	}
	insertRun := `INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,lease_until) VALUES($1,'a',$2,'running','g','e','sig','[]','infinity')`
	if _, err := db.Exec(insertRun, "rp", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cache_events(id,workspace,keys) VALUES('between','a',ARRAY['project:other:requirements'])`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insertRun, "rq", "q"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE analysis_runs SET stage='finding_pairs' WHERE id='rp'; UPDATE analysis_runs SET stage='finding_pairs' WHERE id='rq'`); err != nil {
		t.Fatal(err)
	}
	batch, err := readEventBatch(context.Background(), db, "a", eventCursor{id: cursorID})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.progress) != 2 || batch.progress[0].ProjectID != "p" || batch.progress[1].ProjectID != "q" {
		t.Fatalf("progress order: %+v", batch.progress)
	}
	replay, err := readEventBatch(context.Background(), db, "a", eventCursor{id: batch.progress[0].CursorID})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.progress) != 1 || replay.progress[0].ProjectID != "q" {
		t.Fatalf("replay after first project snapshot lost next progress: %+v", replay.progress)
	}
}
