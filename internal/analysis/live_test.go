package analysis

import (
	"context"
	"testing"
)

func TestSummaryReturnsRevisionAndOmitsDetails(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	service := New(db, retriever, nil, "embed-a")
	defer service.Close()
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary(context.Background(), "a", "p", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ID != run.ID || summary.Revision == "" {
		t.Fatalf("identity/revision: id=%q revision=%q", summary.ID, summary.Revision)
	}
	if len(summary.Sources) != 0 || len(summary.Findings) != 0 || len(summary.Pairs) != 0 {
		t.Fatalf("summary includes details: sources=%d findings=%d pairs=%d", len(summary.Sources), len(summary.Findings), len(summary.Pairs))
	}
	metadata, err := service.Metadata(context.Background(), "a", "p", run.ID)
	if err != nil || metadata.ID != run.ID || metadata.ProjectID != "p" || metadata.SelectionProfile == "" {
		t.Fatalf("lazy metadata: %+v %v", metadata, err)
	}
}

func TestAnalysisRevisionTracksVisibleChangesButNotLease(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	service := New(db, retriever, nil, "embed-a")
	defer service.Close()
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	var revision string
	if err = db.QueryRow(`SELECT revision::text FROM analysis_runs WHERE id=$1`, run.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != "1" {
		t.Fatalf("initial revision = %s", revision)
	}
	var events int
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys=ARRAY['project:p:analysis']`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE analysis_runs SET lease_until=clock_timestamp()+interval '30 seconds' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	var afterLease string
	if err = db.QueryRow(`SELECT revision::text FROM analysis_runs WHERE id=$1`, run.ID).Scan(&afterLease); err != nil {
		t.Fatal(err)
	}
	if afterLease != revision {
		t.Fatalf("lease changed revision: %s -> %s", revision, afterLease)
	}
	if _, err = db.Exec(`UPDATE analysis_runs SET stage='finding_pairs' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	var afterVisible string
	if err = db.QueryRow(`SELECT revision::text FROM analysis_runs WHERE id=$1`, run.ID).Scan(&afterVisible); err != nil {
		t.Fatal(err)
	}
	if afterVisible != "2" {
		t.Fatalf("visible change revision = %s", afterVisible)
	}
	var nextEvents int
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys=ARRAY['project:p:analysis']`).Scan(&nextEvents); err != nil {
		t.Fatal(err)
	}
	if nextEvents != events+1 {
		t.Fatalf("analysis events = %d, want %d", nextEvents, events+1)
	}
}
