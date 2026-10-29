package analysis

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/rag"
	"github.com/VolkovDani/project-creator/internal/storage"
)

type fakeEmbedding struct{ calls atomic.Int32 }

func (e *fakeEmbedding) Profile() string { return "embed-a" }
func (e *fakeEmbedding) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls.Add(1)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 1024)
		out[i][0] = 1
	}
	return out, nil
}

type fakeGenerator struct {
	calls            atomic.Int32
	profile          string
	fail, invalid    bool
	started, release chan struct{}
}

func (g *fakeGenerator) Profile() string { return g.profile }
func (g *fakeGenerator) Compare(ctx context.Context, pair Pair) (Finding, error) {
	g.calls.Add(1)
	if g.started != nil {
		select {
		case g.started <- struct{}{}:
		default:
		}
	}
	if g.release != nil {
		select {
		case <-g.release:
		case <-ctx.Done():
			return Finding{}, ctx.Err()
		}
	}
	if g.fail {
		return Finding{}, errors.New("fake generator failure")
	}
	verdict := "compatible"
	if strings.Contains(pair.Left.Text, "картой") && strings.Contains(pair.Right.Text, "наличными") {
		verdict = "collision"
	}
	f := Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: verdict, Explanation: "Различающиеся правила оплаты требуют проверки условий."}
	if g.invalid {
		f.LeftID = "invented"
	}
	return f, nil
}
func analysisDB(t *testing.T) (*sql.DB, *rag.Service, *fakeEmbedding) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required")
		}
		t.Skip("PostgreSQL integration requires TEST_DATABASE_URL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var random [12]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "analysis_" + hex.EncodeToString(random[:])
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','a','Q','G','','now','now'),('secret','b','Secret','G','','now','now');INSERT INTO requirements VALUES('left','a','p','Оплата разрешена только картой.',1,'now','now'),('right','a','p','Оплата разрешена только наличными.',1,'now','now'),('foreign','a','q','Other project',1,'now','now'),('hidden','b','secret','Other workspace',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	embed := &fakeEmbedding{}
	return db, rag.New(db, embed), embed
}
func ready(t *testing.T, retriever *rag.Service) {
	t.Helper()
	for _, id := range []string{"left", "right"} {
		state, err := retriever.Index(context.Background(), "a", "p", id)
		if err != nil || state.Status != "ready" {
			t.Fatalf("index %s: %+v %v", id, state, err)
		}
	}
}
func TestAnalysisCollisionCompatibleAndSources(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a"}
	s := New(db, retriever, gen, "embed-a")
	run, err := s.Start(context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.Outcome != "possible_collision" || run.Stale || len(run.Findings) != 1 {
		t.Fatalf("run: %+v %v", run, err)
	}
	if len(run.Sources) != 2 || run.Sources[0].ID != "left" || run.Findings[0].RightVersion != 1 {
		t.Fatal(run)
	}
	latest, err := New(db, retriever, gen, "embed-a").Latest(context.Background(), "a", "p")
	if err != nil || latest == nil || latest.ID != run.ID || latest.Stale {
		t.Fatalf("restart latest: %+v %v", latest, err)
	}
	if _, err = db.Exec(`UPDATE requirements SET text='Пользователь видит чек после оплаты.',version=2 WHERE id='right'`); err != nil {
		t.Fatal(err)
	}
	ready(t, retriever)
	compatible, err := s.Start(context.Background(), "a", "p")
	if err != nil || compatible.Outcome != "no_collisions_found" || compatible.Findings[0].Verdict != "compatible" {
		t.Fatalf("compatible: %+v %v", compatible, err)
	}
}
func TestAnalysisAuthorizationAndPendingIndex(t *testing.T) {
	db, retriever, embed := analysisDB(t)
	gen := &fakeGenerator{profile: "gen-a"}
	s := New(db, retriever, gen, "embed-a")
	if _, err := s.Start(context.Background(), "b", "p"); err != ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.Latest(context.Background(), "b", "p"); err != ErrNotFound {
		t.Fatal(err)
	}
	if embed.calls.Load() != 0 || gen.calls.Load() != 0 {
		t.Fatal("provider invoked before access check")
	}
	run, err := s.Start(context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.Outcome != "insufficient" || gen.calls.Load() != 0 || embed.calls.Load() != 0 {
		t.Fatalf("unindexed: %+v %v", run, err)
	}
	run, err = New(db, retriever, nil, "").Start(context.Background(), "a", "p")
	if err != nil || run.Status != "failed" || run.ErrorCode != "NOT_CONFIGURED" {
		t.Fatalf("disabled: %+v %v", run, err)
	}
}
func TestAnalysisStaleVersionsCreationDeletionAndProfiles(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a"}
	s := New(db, retriever, gen, "embed-a")
	run, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	for _, sqlText := range []string{`UPDATE requirements SET text='Changed',version=2 WHERE id='left'`, `INSERT INTO requirements VALUES('added','a','p','New',1,'now','now')`, `DELETE FROM requirements WHERE id='right'`} {
		if _, err = db.Exec(sqlText); err != nil {
			t.Fatal(err)
		}
		latest, err := s.Latest(context.Background(), "a", "p")
		if err != nil || !latest.Stale || latest.Sources[0].Text != run.Sources[0].Text {
			t.Fatalf("stale snapshot: %+v %v", latest, err)
		}
	}
	for _, changed := range []*Service{New(db, retriever, &fakeGenerator{profile: "gen-b"}, "embed-a"), New(db, retriever, gen, "embed-b")} {
		latest, err := changed.Latest(context.Background(), "a", "p")
		if err != nil || !latest.Stale {
			t.Fatalf("profile stale: %+v %v", latest, err)
		}
	}
}
func TestAnalysisInvalidOutputRetryAndTimeout(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a", invalid: true}
	s := New(db, retriever, gen, "embed-a")
	run, err := s.Start(context.Background(), "a", "p")
	if err != nil || run.Status != "failed" || run.ErrorCode != "INVALID_OUTPUT" {
		t.Fatalf("invalid output: %+v %v", run, err)
	}
	gen.invalid = false
	gen.fail = true
	run, err = s.Start(context.Background(), "a", "p")
	if err != nil || run.ErrorCode != "GENERATION_FAILED" {
		t.Fatalf("failure: %+v %v", run, err)
	}
	gen.fail = false
	run, err = s.Start(context.Background(), "a", "p")
	if err != nil || run.Status != "completed" {
		t.Fatalf("retry: %+v %v", run, err)
	}
	gen.release = make(chan struct{})
	s.timeout = 20 * time.Millisecond
	run, err = s.Start(context.Background(), "a", "p")
	if err != nil || run.Status != "failed" || run.ErrorCode != "TIMEOUT" {
		t.Fatalf("timeout persisted: %+v %v", run, err)
	}
}
func TestConcurrentAnalysisAndExpiredAttempt(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a", started: make(chan struct{}, 1), release: make(chan struct{})}
	s := New(db, retriever, gen, "embed-a")
	result := make(chan Run, 1)
	fail := make(chan error, 1)
	go func() { run, err := s.Start(context.Background(), "a", "p"); result <- run; fail <- err }()
	select {
	case <-gen.started:
	case <-time.After(3 * time.Second):
		t.Fatal("generation did not start")
	}
	running, err := s.Start(context.Background(), "a", "p")
	if err != nil || running.Status != "running" || gen.calls.Load() != 1 {
		t.Fatalf("concurrent claim: %+v %v", running, err)
	}
	if _, err = db.Exec(`UPDATE analysis_runs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, running.ID); err != nil {
		t.Fatal(err)
	}
	replacement, err := New(db, retriever, &fakeGenerator{profile: "gen-a"}, "embed-a").Start(context.Background(), "a", "p")
	if err != nil || replacement.ID == running.ID || replacement.Status != "completed" {
		t.Fatalf("expired retry: %+v %v", replacement, err)
	}
	close(gen.release)
	old := <-result
	if err = <-fail; err != nil || old.Status != "failed" || old.ErrorCode != "INTERRUPTED" {
		t.Fatalf("stale completion: %+v %v", old, err)
	}
	latest, err := s.Latest(context.Background(), "a", "p")
	if err != nil || latest.ID != replacement.ID {
		t.Fatalf("old run replaced new: %+v %v", latest, err)
	}
}

func TestAnalysisChangingSourcesDuringGenerationIsStale(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a", started: make(chan struct{}, 1), release: make(chan struct{})}
	s := New(db, retriever, gen, "embed-a")
	type response struct {
		run Run
		err error
	}
	result := make(chan response, 1)
	go func() { r, e := s.Start(context.Background(), "a", "p"); result <- response{r, e} }()
	select {
	case <-gen.started:
	case <-time.After(3 * time.Second):
		t.Fatal("generator did not start")
	}
	if _, err := db.Exec(`UPDATE requirements SET text='Revised',version=2 WHERE id='right'`); err != nil {
		t.Fatal(err)
	}
	close(gen.release)
	out := <-result
	if out.err != nil || !out.run.Stale || out.run.Sources[1].Version != 1 {
		t.Fatalf("stale result must retain old version: %+v %v", out.run, out.err)
	}
}
func TestBoundedAnalysisIsExplicitlyInsufficient(t *testing.T) {
	db, retriever, embed := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a"}
	s := New(db, retriever, gen, "embed-a")
	if _, err := db.Exec(`INSERT INTO requirements SELECT 'extra-'||n,'a','p','Bounded requirement',1,'now','now' FROM generate_series(1,101) n`); err != nil {
		t.Fatal(err)
	}
	before := embed.calls.Load()
	run, err := s.Start(context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.Outcome != "insufficient" || len(run.Sources) != 100 || gen.calls.Load() != 0 || embed.calls.Load() != before || run.Stale {
		t.Fatalf("bounded analysis: %+v %v", run, err)
	}
}

func TestAnalysisDiagnosticsPersistMissingIndexAndSeparateModelOutcome(t *testing.T) {
	for _, mode := range []string{"missing", "old-version", "old-profile", "failed", "ready", "model-insufficient"} {
		t.Run(mode, func(t *testing.T) {
			db, retriever, _ := analysisDB(t)
			ready(t, retriever)
			var statement string
			switch mode {
			case "missing":
				statement = `DELETE FROM requirement_index WHERE requirement_id='right'`
			case "old-version":
				statement = `UPDATE requirements SET version=2 WHERE id='right'`
			case "old-profile":
				statement = `UPDATE requirement_index SET profile='old' WHERE requirement_id='right'`
			case "failed":
				statement = `UPDATE requirement_index SET status='failed',error_code='EMBEDDING_FAILED' WHERE requirement_id='right'`
			}
			if statement != "" {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			gen := &fakeGenerator{profile: "gen-a"}
			var provider Generator = gen
			if mode == "model-insufficient" {
				provider = generatorFunc(func(_ context.Context, pair Pair) (Finding, error) {
					return Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "insufficient", Explanation: "Missing conditions"}, nil
				})
			}
			service := New(db, retriever, provider, "embed-a")
			run, err := service.Start(context.Background(), "a", "p")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "ready":
				if run.ReasonCode != "" || run.Outcome != "possible_collision" {
					t.Fatal(run)
				}
			case "model-insufficient":
				if run.ReasonCode != "MODEL_INSUFFICIENT" || len(run.MissingRequirementIDs) != 0 {
					t.Fatal(run)
				}
			default:
				if run.Status != "completed" || run.Outcome != "insufficient" || run.ReasonCode != "INDEX_INCOMPLETE" || len(run.MissingRequirementIDs) != 1 || run.MissingRequirementIDs[0] != "right" || gen.calls.Load() != 0 {
					t.Fatal(run)
				}
			}
			latest, err := New(db, retriever, provider, "embed-a").Latest(context.Background(), "a", "p")
			if err != nil || latest == nil || latest.ReasonCode != run.ReasonCode || strings.Join(latest.MissingRequirementIDs, ",") != strings.Join(run.MissingRequirementIDs, ",") {
				t.Fatalf("persisted: %+v %v", latest, err)
			}
		})
	}
}
