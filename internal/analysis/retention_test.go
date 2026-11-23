package analysis

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHistoryRetentionDeadlineAndScope(t *testing.T) {
	db, _, _ := analysisDB(t)
	s := &Service{db: db}
	ctx := context.Background()
	run, err := s.Start(ctx, "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ScheduleDeletion(ctx, "a", "p", run.ID); !errors.Is(err, ErrDeletionUnavailable) {
		t.Fatalf("marked running run: %v", err)
	}
	if _, err = s.Cancel(ctx, "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteMarked(ctx, "a", "p", run.ID); !errors.Is(err, ErrDeletionUnavailable) {
		t.Fatalf("deleted unmarked run: %v", err)
	}
	if s.RetentionDays() != 30 {
		t.Fatalf("default retention = %d", s.RetentionDays())
	}
	s.SetRetentionDays(4)
	for _, scope := range [][2]string{{"a", "q"}, {"b", "secret"}, {"b", "p"}} {
		if err = s.ScheduleDeletion(ctx, scope[0], scope[1], run.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("schedule across scope %v: %v", scope, err)
		}
	}
	before := time.Now().Add(4 * 24 * time.Hour)
	if err = s.ScheduleDeletion(ctx, "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	after := time.Now().Add(4 * 24 * time.Hour)
	history, err := s.History(ctx, "a", "p", 10, 0)
	if err != nil || len(history) != 1 || history[0].DeletionScheduledFor == nil {
		t.Fatalf("marked run missing from history: %+v, %v", history, err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, *history[0].DeletionScheduledFor)
	if err != nil || deadline.Before(before.Add(-time.Second)) || deadline.After(after.Add(time.Second)) {
		t.Fatalf("deadline %s outside four-day window: %v", deadline, err)
	}
	s.SetRetentionDays(7)
	if err = s.ScheduleDeletion(ctx, "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summary(ctx, "a", "p", run.ID)
	if err != nil || summary.DeletionScheduledFor == nil || *summary.DeletionScheduledFor != *history[0].DeletionScheduledFor || summary.Revision != history[0].Revision {
		t.Fatalf("repeated mark changed deadline/revision: %+v, %v", summary, err)
	}
	if _, err = s.Retry(ctx, "a", "p", run.ID); !errors.Is(err, ErrRetryUnavailable) {
		t.Fatalf("retry revived marked run: %v", err)
	}
	for _, scope := range [][2]string{{"a", "q"}, {"b", "secret"}, {"b", "p"}} {
		if err = s.DeleteMarked(ctx, scope[0], scope[1], run.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("delete across scope %v: %v", scope, err)
		}
	}
	if err = s.DeleteMarked(ctx, "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, "a", "p", run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("manually deleted run remains: %v", err)
	}
	var events int
	if err = db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys @> ARRAY['project:p:analysis-history']`).Scan(&events); err != nil || events != 2 {
		t.Fatalf("retention event count = %d, want 2: %v", events, err)
	}
}

func TestPurgeExpiredPreservesOtherRunAndCopiedResults(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	s := testService(t, db, retriever, &fakeGenerator{profile: "gen-a"}, "embed-a")
	ctx := context.Background()
	original, err := s.startAndWait(t, ctx, "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	copyRun, err := s.startAndWait(t, ctx, "a", "p")
	if err != nil || len(copyRun.Pairs) != 1 || copyRun.ReusedPairs != 1 {
		t.Fatalf("expected cached result: %+v, %v", copyRun, err)
	}
	for _, run := range []Run{original, copyRun} {
		if err = s.ScheduleDeletion(ctx, "a", "p", run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`UPDATE analysis_runs SET deletion_scheduled_for=now()-interval '1 second' WHERE id=$1`, original.ID); err != nil {
		t.Fatal(err)
	}
	// History reads and administrative changes serialize through the project row.
	// A sweeper must skip a busy project and retry it on a subsequent pass.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, "a", "p", true); err != nil {
		t.Fatal(err)
	}
	sweepCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err = s.PurgeExpired(sweepCtx); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_runs WHERE id=$1)`, original.ID).Scan(&exists); err != nil || !exists {
		t.Fatalf("sweeper removed a run while the project was locked: %v", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = s.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, "a", "p", original.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired run remains: %v", err)
	}
	remaining, err := s.Get(ctx, "a", "p", copyRun.ID)
	if err != nil || len(remaining.Sources) != 2 || len(remaining.Pairs) != 1 || remaining.Pairs[0].ReusedFrom != original.Pairs[0].ID || remaining.Pairs[0].Explanation != original.Pairs[0].Explanation {
		t.Fatalf("remaining run lost source or cache provenance: %+v, %v", remaining, err)
	}
	var children int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM analysis_sources WHERE run_id=$1) + (SELECT count(*) FROM analysis_pairs WHERE run_id=$1)`, original.ID).Scan(&children); err != nil || children != 0 {
		t.Fatalf("deleted run has %d child rows: %v", children, err)
	}
}
