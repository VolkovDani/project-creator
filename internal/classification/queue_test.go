package classification

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func queueDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required")
		}
		t.Skip("set TEST_DATABASE_URL")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("classification_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer admin.Close()
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('other','b','B','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','first',1,'now','now'),('secret','b','other','private',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	return db
}

type blockingClassifier struct {
	calls   chan string
	release chan bool
	profile string
}

func (b blockingClassifier) Profile() string         { return b.profile }
func (b blockingClassifier) Digest() string          { return "test-digest" }
func (b blockingClassifier) Model() string           { return "local-test" }
func (b blockingClassifier) QuestionVersion() string { return "q1" }
func (b blockingClassifier) Classify(ctx context.Context, text string) (Prediction, error) {
	select {
	case b.calls <- text:
	case <-ctx.Done():
		return Prediction{}, ctx.Err()
	}
	select {
	case success := <-b.release:
		if !success {
			return Prediction{}, fmt.Errorf("secret raw model error")
		}
	case <-ctx.Done():
		return Prediction{}, ctx.Err()
	}
	score := 0.55
	return Prediction{Status: "needs_review", Semantic: "unknown", Representation: "unknown", Reason: "uncalibrated", Alternatives: []Alternative{{RepresentationType: "test_case", Score: &score}}}, nil
}

func TestClassificationWorkerFencesAndKeepsConfirmation(t *testing.T) {
	db := queueDatabase(t)
	ctx := context.Background()
	fake := blockingClassifier{make(chan string, 1), make(chan bool, 1), "v1"}
	s := New(db, fake)
	if err := s.Start([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	awaitCall := func(want string) {
		t.Helper()
		select {
		case got := <-fake.calls:
			if got != want {
				t.Fatalf("call %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("missing call for %s", want)
		}
	}
	awaitCall("first")
	if _, err := db.Exec(`UPDATE requirements SET version=2,text='changed' WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	fake.release <- true
	awaitCall("changed")
	confirmed, fields, err := Confirm(ctx, db, "a", "p", Entry{ID: "r", Version: 2, Semantic: "functional_requirement", Representation: "scenario"})
	if err != nil || len(fields) > 0 || confirmed == nil {
		t.Fatalf("%v %v %v", confirmed, fields, err)
	}
	fake.release <- true
	if n, err := s.Backfill(ctx, "a", "p"); err != nil || n != 0 {
		t.Fatalf("backfill=%d %v", n, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		changed, err := s.Retry(ctx, "a", "p", "r", 2, true)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("could not reanalyze")
		}
		time.Sleep(10 * time.Millisecond)
	}
	awaitCall("changed")
	fake.release <- true
	var history []Entry
	for {
		history, err = History(ctx, db, "a", "p", "r", 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("history: %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if history[0].Reason != "uncalibrated" || history[0].Version != 2 || history[0].Digest != "test-digest" || len(history[0].Alternatives) != 1 || history[1].Status != "confirmed" {
		t.Fatalf("%+v", history)
	}
	current, err := Get(ctx, db, "a", "p", "r")
	if err != nil || current.Status != "confirmed" || current.Representation != "scenario" {
		t.Fatalf("%+v %v", current, err)
	}
	if _, fields, err = Confirm(ctx, db, "a", "p", Entry{ID: "r", Version: 2, Revision: current.Revision, Semantic: "business_rule", Representation: "statement"}); err != nil || len(fields) > 0 {
		t.Fatalf("post-reanalysis confirmation: %v %v", fields, err)
	}
	if _, err = s.Backfill(ctx, "a", "other"); err != ErrUnavailable {
		t.Fatalf("foreign project %v", err)
	}
	if n, err := s.Backfill(ctx, "a", "p"); err != nil || n != 0 {
		t.Fatalf("duplicate %d %v", n, err)
	}
}

func TestClassificationQueueRetryAndIdentity(t *testing.T) {
	db := queueDatabase(t)
	ctx := context.Background()
	fake := blockingClassifier{make(chan string, 1), make(chan bool, 1), "A"}
	s := New(db, fake)
	// Activate the profile without starting the background loop to drive recovery deterministically.
	_, err := db.Exec(`INSERT INTO classification_profiles VALUES('a','A','local-test','test-digest','q1','2')`)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.Backfill(ctx, "a", "p"); err != nil || n != 1 {
		t.Fatalf("backfill %d %v", n, err)
	}
	take := func() *job {
		t.Helper()
		j, e := s.claim(ctx, []string{"a"})
		if e != nil || j == nil {
			t.Fatalf("claim %+v %v", j, e)
		}
		return j
	}
	expire := func() {
		t.Helper()
		if _, e := db.Exec(`UPDATE classification_jobs SET available_at=clock_timestamp()-interval '1 second' WHERE requirement_id='r'; UPDATE classification_call SET lease_until=clock_timestamp()-interval '1 second'`); e != nil {
			t.Fatal(e)
		}
	}
	first := take()
	if other, e := s.claim(ctx, []string{"a"}); e != nil || other != nil {
		t.Fatalf("overlapping: %+v %v", other, e)
	}
	expire()
	second := take()
	if second.attempt == first.attempt || second.attempts != 2 {
		t.Fatal(second)
	}
	if err = s.finish(ctx, *first, Prediction{}, "CLASSIFIER_UNAVAILABLE"); err != nil {
		t.Fatal(err)
	}
	if err = s.finish(ctx, *second, Prediction{}, "CLASSIFIER_UNAVAILABLE"); err != nil {
		t.Fatal(err)
	}
	if j, e := s.claim(ctx, []string{"a"}); e != nil || j != nil {
		t.Fatalf("backoff: %+v %v", j, e)
	}
	expire()
	third := take()
	expire()
	if j, e := s.claim(ctx, []string{"a"}); e != nil || j != nil {
		t.Fatalf("exhausted: %+v %v", j, e)
	}
	if err = s.finish(ctx, *third, Prediction{}, "STALE_COMPLETION"); err != nil {
		t.Fatal(err)
	}
	progress, e := s.Progress(ctx, "a", "p")
	if e != nil || progress.Failed != 1 || progress.Total != 1 || !progress.Configured {
		t.Fatalf("%+v %v", progress, e)
	}
	list, e := s.List(ctx, "a", "p", nil, nil, nil, 10, 0, false)
	if e != nil || len(list) != 1 || list[0].Text != "" || list[0].ErrorCode != "INTERRUPTED" {
		t.Fatalf("%+v %v", list, e)
	}
	if n, e := s.RetryFailed(ctx, "a", "p"); n != 1 || e != nil {
		t.Fatalf("retry %d %v", n, e)
	}
	if n, e := s.RetryFailed(ctx, "a", "p"); n != 0 || e != nil {
		t.Fatal("non-idempotent retry", n, e)
	}
	if rows, e := History(ctx, db, "a", "p", "r", 10, 0); e != nil || len(rows) != 0 {
		t.Fatalf("failure became a model decision %v %v", rows, e)
	}
	j := take()
	decision := Prediction{Status: "needs_review", Semantic: "unknown", Representation: "unknown", Reason: "uncalibrated"}
	if e = s.finish(ctx, *j, decision, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE classification_profiles SET profile='B',digest='another' WHERE workspace='a'`); e != nil {
		t.Fatal(e)
	}
	current, e := Get(ctx, db, "a", "p", "r")
	if e != nil || current.Status != "stale" {
		t.Fatalf("%+v %v", current, e)
	}
	fake.profile = "B"
	s = New(db, fake)
	if n, e := s.Backfill(ctx, "a", "p"); n != 1 || e != nil {
		t.Fatalf("%d %v", n, e)
	}
	if e = s.finish(ctx, *take(), decision, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE classification_profiles SET profile='A',digest='test-digest' WHERE workspace='a'`); e != nil {
		t.Fatal(e)
	}
	current, e = Get(ctx, db, "a", "p", "r")
	if e != nil || current.Status != "needs_review" || current.Profile != "A" {
		t.Fatalf("restored profile: %+v %v", current, e)
	}
	rows, e := s.ListByIDs(ctx, "a", "p", []string{"secret", "r"})
	if e != nil || len(rows) != 1 || rows[0].Text != "first" {
		t.Fatalf("%+v %v", rows, e)
	}
}

func TestManualModeAndSourceEvents(t *testing.T) {
	db := queueDatabase(t)
	ctx := context.Background()
	s := New(db, nil)
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if n, e := s.RetryFailed(ctx, "a", "p"); n != 0 || e != ErrNotConfigured {
		t.Fatalf("%d %v", n, e)
	}
	if n, e := s.RetryFailed(ctx, "a", "other"); n != 0 || e != ErrUnavailable {
		t.Fatalf("%d %v", n, e)
	}
	progress, e := s.Progress(ctx, "a", "p")
	if e != nil || progress.Total != 1 || progress.Unclassified != 1 || progress.Configured {
		t.Fatalf("%+v %v", progress, e)
	}
	g, e := s.Groups(ctx, "a", "p")
	if e != nil || len(g) != 1 || g[0] != (Group{"unknown", "unknown", 1}) {
		t.Fatalf("%v %v", g, e)
	}
	v, e := Get(ctx, db, "a", "p", "r")
	if e != nil || v.ErrorCode != "NOT_CONFIGURED" {
		t.Fatalf("%+v %v", v, e)
	}
	var has bool
	if e = db.QueryRow(`SELECT EXISTS(SELECT FROM cache_events WHERE workspace='a' AND keys=ARRAY['project:p:classifications','project:p:classification:r'])`).Scan(&has); e != nil || !has {
		t.Fatal("missing scoped classification invalidation on unconfigured create", e)
	}
}

func TestLargeBackfillStagesBoundedBatches(t *testing.T) {
	db := queueDatabase(t)
	ctx := context.Background()
	_, e := db.Exec(`INSERT INTO requirements SELECT 'new-'||i,'a','p','source',1,'now','now' FROM generate_series(1,140)i;
 INSERT INTO classification_profiles VALUES('a','A','local-test','test-digest','q1','2')`)
	if e != nil {
		t.Fatal(e)
	}
	fake := blockingClassifier{make(chan string, 1), make(chan bool, 1), "A"}
	s := New(db, fake)
	n, e := s.Backfill(ctx, "a", "p")
	if e != nil || n < 1 || n > 64 {
		t.Fatalf("first batch %d %v", n, e)
	}
	// A worker created after the mutation can resume the stored request.
	s = New(db, fake)
	for i := 0; i < 4; i++ {
		if e = s.continueBackfill(ctx, []string{"a"}); e != nil {
			t.Fatal(e)
		}
	}
	p, e := s.Progress(ctx, "a", "p")
	if e != nil || p.Queued != 141 {
		t.Fatalf("%+v %v", p, e)
	}
	n, e = s.Backfill(ctx, "a", "p")
	if e != nil || n != 0 {
		t.Fatalf("completed backfill repeated %d %v", n, e)
	}
}

func TestNewSourceWaitsForOldInferenceAndUsesCurrentMetadata(t *testing.T) {
	db := queueDatabase(t)
	ctx := context.Background()
	fake := blockingClassifier{profile: "A"}
	s := New(db, fake)
	if _, e := db.Exec(`INSERT INTO classification_profiles VALUES('a','A','local-test','test-digest','q1','2')`); e != nil {
		t.Fatal(e)
	}
	if n, e := s.Backfill(ctx, "a", "p"); n != 1 || e != nil {
		t.Fatal(n, e)
	}
	previous, e := s.claim(ctx, []string{"a"})
	if e != nil || previous == nil {
		t.Fatal(e)
	}
	decision := Prediction{Status: "suggested", Semantic: "business_rule", Representation: "statement"}
	if e = s.finish(ctx, *previous, decision, ""); e != nil {
		t.Fatal(e)
	}
	if ok, e := s.Retry(ctx, "a", "p", "r", 1, true); !ok || e != nil {
		t.Fatal(ok, e)
	}
	running, e := s.claim(ctx, []string{"a"})
	if e != nil || running == nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE requirements SET text='edited',version=2 WHERE id='r'`); e != nil {
		t.Fatal(e)
	}
	current, e := Get(ctx, db, "a", "p", "r")
	if e != nil || current.Version != 2 || current.Representation != "unknown" || current.Source != "" || current.Status != "queued" {
		t.Errorf("current queue borrows stale decision: %+v %v", current, e)
	}
	if j, e := s.claim(ctx, []string{"a"}); e != nil || j != nil {
		t.Fatalf("second process claimed while previous inference still running %+v %v", j, e)
	}
	if e = s.finish(ctx, *running, decision, ""); e != nil {
		t.Fatal(e)
	}
	j, e := s.claim(ctx, []string{"a"})
	if e != nil || j == nil || j.version != 2 {
		t.Fatal(j, e)
	}
}

func TestGracefulRestartRequeuesInFlightSource(t *testing.T) {
	db := queueDatabase(t)
	ctx := context.Background()
	fake := blockingClassifier{make(chan string, 1), make(chan bool, 1), "A"}
	s := New(db, fake)
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	select {
	case <-fake.calls:
	case <-time.After(5 * time.Second):
		s.Close()
		t.Fatal("no call")
	}
	s.Close()
	current, e := Get(ctx, db, "a", "p", "r")
	if e != nil || current.Status != "queued" {
		t.Fatalf("graceful shutdown %+v %v", current, e)
	}
	next := New(db, fake)
	if e = next.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	select {
	case <-fake.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("restart did not resume")
	}
}
