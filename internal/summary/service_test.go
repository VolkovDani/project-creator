package summary

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/storage"
)

type fakeGenerator struct {
	calls atomic.Int32
	text  string
	block chan struct{}
}

func (g *fakeGenerator) Profile() string { return "summary-v1" }
func (g *fakeGenerator) Provenance() string {
	return `{"model":"fake","digest":"pinned","instruction":"v1"}`
}
func (g *fakeGenerator) Summarize(ctx context.Context, source analysis.Source) (string, error) {
	g.calls.Add(1)
	if g.block != nil {
		select {
		case <-g.block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return g.text, nil
}
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	admin, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	var b [8]byte
	rand.Read(b[:])
	schema := "summary_" + hex.EncodeToString(b[:])
	if _, e = admin.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := storage.Open(u.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	if _, e = db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('secret','b','Secret','G','','now','now');INSERT INTO requirements VALUES('r','a','p','Original requirement with meaningful limits: no guest access, except public tickets.',1,'now','now'),('hidden','b','secret','Private',1,'now','now')`); e != nil {
		t.Fatal(e)
	}
	return db
}
func waitState(t *testing.T, s *Service, w, p, id, status string) State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, e := s.ByIDs(context.Background(), w, p, []string{id})
		if e != nil {
			t.Fatal(e)
		}
		if rows[0].Status == status {
			return rows[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s state", status)
	return State{}
}
func TestAutomaticSummaryVersionIsolationAndRetry(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: "No guest access, except public tickets."}
	s := New(db, g)
	defer s.Close()
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	v := waitState(t, s, "a", "p", "r", "ready")
	if v.Version != 1 || v.Text != g.text || v.Provenance == "" {
		t.Fatalf("state %+v", v)
	}
	if _, e := s.ByIDs(context.Background(), "a", "secret", []string{"hidden"}); e == nil {
		t.Fatal("foreign workspace read")
	}
	if g.calls.Load() != 1 {
		t.Fatal("foreign model call or duplicate")
	}
	if _, e := db.Exec(`UPDATE requirements SET text='Updated original',version=2 WHERE id='r'`); e != nil {
		t.Fatal(e)
	}
	v = waitState(t, s, "a", "p", "r", "ready")
	if v.Version != 2 {
		t.Fatalf("stale summary %+v", v)
	}
	if _, e := s.Retry(context.Background(), "a", "p", "r"); e != nil {
		t.Fatal(e)
	}
	time.Sleep(30 * time.Millisecond)
	if g.calls.Load() != 2 {
		t.Fatal("ready summary regenerated")
	}
}
func TestEditAndCancelFenceLateSummary(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: "Derived", block: make(chan struct{})}
	s := New(db, g)
	defer s.Close()
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, "a", "p", "r", "running")
	if _, e := s.Cancel(context.Background(), "a", "p", "r"); e != nil {
		t.Fatal(e)
	}
	close(g.block)
	waitState(t, s, "a", "p", "r", "cancelled")
	if _, e := s.Retry(context.Background(), "a", "p", "r"); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, "a", "p", "r", "ready")
}
func TestInvalidSummaryFailsWithoutTruncation(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: string(make([]byte, 351))}
	s := New(db, g)
	defer s.Close()
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	v := waitState(t, s, "a", "p", "r", "failed")
	if v.Text != "" || v.ErrorCode != "INVALID_OUTPUT" {
		t.Fatalf("invalid accepted %+v", v)
	}
}

type captureClassifier struct {
	called      atomic.Int32
	left, right string
}

func (c *captureClassifier) Profile() string { return "classifier-v1" }
func (c *captureClassifier) Classify(_ context.Context, p analysis.Pair) (analysis.Preclassification, error) {
	c.called.Add(1)
	c.left = p.Left.Text
	c.right = p.Right.Text
	return analysis.Preclassification{Decision: "related", Scores: map[string]float64{"related": 1, "unrelated": 0, "uncertain": 0}}, nil
}
func TestScopedClassifierUsesSummariesAndRejectsStaleOrForeignSources(t *testing.T) {
	db := testDB(t)
	db.Exec(`INSERT INTO requirements VALUES('second','a','p','Second original',1,'now','now')`)
	g := &fakeGenerator{text: "Summary with no omitted constraints"}
	s := New(db, g)
	defer s.Close()
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, "a", "p", "r", "ready")
	waitState(t, s, "a", "p", "second", "ready")
	next := &captureClassifier{}
	c := s.Classifier(next).(*Classifier)
	pair := analysis.Pair{Left: analysis.Source{ID: "r", Version: 1, Text: "Original requirement with meaningful limits: no guest access, except public tickets."}, Right: analysis.Source{ID: "second", Version: 1, Text: "Second original"}}
	if _, e := c.ClassifyScoped(context.Background(), "a", "p", pair); e != nil {
		t.Fatal(e)
	}
	if next.left != g.text || next.right != g.text || pair.Right.Text != "Second original" {
		t.Fatal("wrong stage source")
	}
	pair.Right.Version = 2
	if _, e := c.ClassifyScoped(context.Background(), "a", "p", pair); e == nil {
		t.Fatal("stale accepted")
	}
	if _, e := c.ClassifyScoped(context.Background(), "b", "p", pair); e == nil {
		t.Fatal("foreign accepted")
	}
	if next.called.Load() != 1 {
		t.Fatal("model call before validation")
	}
}
func TestSummaryOutboxRollsBackAndPreservesHistory(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: "Derived"}
	s := New(db, g)
	defer s.Close()
	s.Start([]string{"a"})
	waitState(t, s, "a", "p", "r", "ready")
	var before int
	db.QueryRow(`SELECT count(*) FROM cache_events WHERE keys=ARRAY['project:p:summaries']`).Scan(&before)
	tx, _ := db.Begin()
	tx.Exec(`UPDATE requirement_summaries SET error_code='rollback' WHERE requirement_id='r'`)
	tx.Rollback()
	var after int
	db.QueryRow(`SELECT count(*) FROM cache_events WHERE keys=ARRAY['project:p:summaries']`).Scan(&after)
	if before != after {
		t.Fatal("uncommitted event")
	}
	db.Exec(`UPDATE requirements SET text='Version two',version=2 WHERE id='r'`)
	waitState(t, s, "a", "p", "r", "ready")
	var old string
	if e := db.QueryRow(`SELECT text FROM requirement_summaries WHERE requirement_id='r' AND version=1 AND status='ready'`).Scan(&old); e != nil || old != "Derived" {
		t.Fatal("old summary history overwritten")
	}
}

func TestProfileChangeFencesOldQueue(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: "Derived"}
	if _, e := db.Exec(`INSERT INTO summary_profiles VALUES('a','old','old');INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status,attempt) VALUES('a','p','r',1,'old','old','running','old-attempt')`); e != nil {
		t.Fatal(e)
	}
	s := New(db, g)
	defer s.Close()
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	var status, attempt string
	if e := db.QueryRow(`SELECT status,attempt FROM requirement_summaries WHERE profile='old'`).Scan(&status, &attempt); e != nil || status != "stale" || attempt != "" {
		t.Fatalf("old queue survived profile change: %s %s %v", status, attempt, e)
	}
}

type finalGenerator struct {
	calls atomic.Int32
	bad   atomic.Bool
}

func (g *finalGenerator) Profile() string { return "full-source-generator-v1" }
func (g *finalGenerator) Compare(_ context.Context, p analysis.Pair) (analysis.Finding, error) {
	g.calls.Add(1)
	if p.Left.Text == "Derived" || p.Right.Text == "Derived" || p.Preclassification == nil {
		g.bad.Store(true)
	}
	return analysis.Finding{LeftID: p.Left.ID, RightID: p.Right.ID, LeftVersion: p.Left.Version, RightVersion: p.Right.Version, Verdict: "compatible", Explanation: "Originals compared"}, nil
}
func TestPipelineSummarizesOnlyLayaAndCachesFinishedComparison(t *testing.T) {
	db := testDB(t)
	if _, e := db.Exec(`INSERT INTO requirements VALUES('second','a','p','Second full original',1,'now','now')`); e != nil {
		t.Fatal(e)
	}
	g := &fakeGenerator{text: "Derived"}
	s := New(db, g)
	defer s.Close()
	s.Start([]string{"a"})
	waitState(t, s, "a", "p", "r", "ready")
	waitState(t, s, "a", "p", "second", "ready")
	classifier := &captureClassifier{}
	final := &finalGenerator{}
	analyzer := analysis.New(db, nil, final, "", s.Classifier(classifier))
	defer analyzer.Close()
	for i := 0; i < 2; i++ {
		r, e := analyzer.StartWithOptions(context.Background(), "a", "p", analysis.Options{Mode: "all"})
		if e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			current, readErr := analyzer.Get(context.Background(), "a", "p", r.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			r = *current
			if r.Status != "running" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("pipeline timeout")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if r.CompletedPairs != 1 {
			t.Fatalf("run %+v", r)
		}
	}
	if final.calls.Load() != 1 || final.bad.Load() || classifier.called.Load() != 1 {
		t.Fatalf("final calls %d bad %v classifier %d", final.calls.Load(), final.bad.Load(), classifier.called.Load())
	}
}

func TestRestartRequiresRetryOnlyForInterruptedStage(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: "Derived"}
	if _, e := db.Exec(`INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status,attempt,lease_until) VALUES('a','p','r',1,'summary-v1','pinned','running','old',clock_timestamp()-interval '1 second')`); e != nil {
		t.Fatal(e)
	}
	s := New(db, g)
	if e := s.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	v := waitState(t, s, "a", "p", "r", "failed")
	if v.ErrorCode != "INTERRUPTED" || g.calls.Load() != 0 {
		t.Fatal("restart silently repeated inference")
	}
	s.Retry(context.Background(), "a", "p", "r")
	waitState(t, s, "a", "p", "r", "ready")
	s.Close()
	next := New(db, g)
	defer next.Close()
	if e := next.Start([]string{"a"}); e != nil {
		t.Fatal(e)
	}
	waitState(t, next, "a", "p", "r", "ready")
	if g.calls.Load() != 1 {
		t.Fatal("restart repeated successful summary")
	}
}
func TestTwoWorkersDoNotDuplicateSummaryInference(t *testing.T) {
	db := testDB(t)
	g := &fakeGenerator{text: "Derived", block: make(chan struct{})}
	first := New(db, g)
	defer first.Close()
	first.Start([]string{"a"})
	waitState(t, first, "a", "p", "r", "running")
	second := New(db, g)
	defer second.Close()
	second.Start([]string{"a"})
	close(g.block)
	waitState(t, first, "a", "p", "r", "ready")
	if g.calls.Load() != 1 {
		t.Fatal("duplicate summary model calls")
	}
}
