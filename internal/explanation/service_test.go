package explanation

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

type generatorStub struct {
	calls   int
	profile string
	source  Source
	after   func()
}

func (g *generatorStub) ExplanationProfile() string { return g.profile }
func (g *generatorStub) ExplainClassification(ctx context.Context, source Source, target Target) (Result, error) {
	g.calls++
	g.source = source
	if _, ok := ctx.Deadline(); !ok {
		panic("missing inference deadline")
	}
	if g.after != nil {
		g.after()
	}
	return Result{Limitations: target.Locale}, nil
}

func database(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required")
		}
		t.Skip("set TEST_DATABASE_URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("explanation_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
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
	return db
}

func TestExplainRequiresScopedCurrentModelEvidence(t *testing.T) {
	db := database(t)
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');
 INSERT INTO requirements VALUES('r','a','p','source',1,'now','now')`)
	gen := &generatorStub{profile: "local-profile"}
	service := New(db, gen)
	req := Request{ProjectID: "p", RequirementID: "r", TextVersion: 1, Target: Target{Semantic: "functional_requirement", Representation: "test_case", Locale: "en"}}
	expect := func(workspace string, req Request, status string, calls int) {
		t.Helper()
		result, got, _, err := service.Explain(context.Background(), workspace, req)
		if err != nil || got != status || gen.calls != calls || (result != nil) != (status == "ready") {
			t.Fatalf("status=%s result=%+v err=%v calls=%d want %s/%d", got, result, err, gen.calls, status, calls)
		}
	}
	revision := func(w string, expected int32) {
		t.Helper()
		got, _, err := service.SuggestionRevision(context.Background(), w, "p", "r")
		if got != expected || err != nil {
			t.Fatalf("eligibility revision=%d, want %d: %v", got, expected, err)
		}
	}
	revision("a", 0)
	expect("b", req, "stale", 0)
	expect("a", req, "unclassified", 0)
	exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type) VALUES('a','p','r',1,1,'confirmed','user','2','business_rule','statement')`)
	req.Revision = 1
	expect("a", req, "unclassified", 0)
	exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type,reason,reanalysis)
 VALUES('a','p','r',1,2,'needs_review','model','2','unknown','unknown','input_limit',true)`)
	expect("a", req, "unclassified", 0)
	exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type,reason,reanalysis,alternatives,model,profile,digest,question_version)
 VALUES('a','p','r',1,3,'needs_review','model','2','unknown','unknown','uncalibrated',true,'[{"SemanticType":"constraint","Score":0.7}]','m','p','d','q')`)
	revision("a", 3)
	revision("b", 0)
	expect("a", req, "stale", 0)
	req.SuggestionRevision = 3
	expect("a", req, "ready", 1)
	if gen.source.Text != "source" || gen.source.Decision.Source != "user" || gen.source.Suggestion.Revision != 3 || gen.source.Suggestion.Model != "m" || len(gen.source.Suggestion.Alternatives) != 1 {
		t.Fatalf("%+v", gen.source)
	}
	expect("a", req, "ready", 1)
	expect("b", req, "stale", 1)
	req.Target.Locale = "ru"
	expect("a", req, "ready", 2)
	// An explicit rerun appends new model evidence while the manual decision stays effective.
	gen.profile = "different"
	gen.after = func() {
		exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type,reanalysis)
 VALUES('a','p','r',1,4,'suggested','model','2','constraint','statement',true)`)
	}
	expect("a", req, "stale", 3)
	revision("a", 4)
	req.SuggestionRevision = 4
	gen.after = nil
	expect("a", req, "ready", 4)
	gen.profile = "new-model"
	gen.after = func() { exec(`UPDATE requirements SET text='edited', version=2 WHERE id='r'`) }
	expect("a", req, "stale", 5)
	revision("a", 0)
	req.TextVersion = 2
	expect("a", req, "unclassified", 5)
}

func TestAdmissionCacheAndWorkspaceWindows(t *testing.T) {
	service := New(nil, nil)
	now := time.Unix(100, 0)
	key := cacheKey{workspace: "a"}
	check := func(key cacheKey, at time.Time, want string) {
		t.Helper()
		_, status := service.acquire(key, at)
		if status != want {
			t.Fatalf("status %q, want %q", status, want)
		}
	}
	check(key, now, "")
	check(key, now, "busy")
	service.release()
	for i := 1; i < callsPerMinute; i++ {
		check(key, now, "")
		service.release()
	}
	check(key, now, "busy")
	other := cacheKey{workspace: "b"}
	check(other, now, "")
	service.release()
	service.store(key, Result{Questions: []string{"original"}})
	cached, status := service.acquire(key, now)
	if status != "ready" {
		t.Fatalf("rate limited cache hit: %s", status)
	}
	cached.Questions[0] = "modified"
	cached, _ = service.acquire(key, now)
	if cached.Questions[0] != "original" {
		t.Fatal("cached result was modified")
	}
	for i := range cacheSize {
		other.profile = fmt.Sprint(i)
		service.store(other, Result{})
	}
	// The oldest result was evicted; it must pass through the workspace limit again.
	check(key, now, "busy")
	check(key, now.Add(time.Minute), "")
	service.release()
	if _, exists := service.recent["b"]; exists {
		t.Fatal("expired workspace retained")
	}
}

func TestInvalidTargets(t *testing.T) {
	service := New(nil, nil)
	r, status, fields, err := service.Explain(context.Background(), "a", Request{Target: Target{Semantic: "statement", Representation: "bogus", Locale: "bad"}})
	if err != nil || r != nil || status != "invalid" || len(fields) != 3 {
		t.Fatalf("%+v %s %+v %v", r, status, fields, err)
	}
	for i, path := range []string{"semanticType", "representationType", "locale"} {
		if fields[i].Code != "INVALID_VALUE" || fields[i].Path[1] != path {
			t.Errorf("%+v", fields[i])
		}
	}
}

func TestCanceledGenerationIsReleasedAndNeverCached(t *testing.T) {
	db := database(t)
	if _, err := db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');
 INSERT INTO requirements VALUES('r','a','p','source',1,'now','now');
 INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type)
 VALUES('a','p','r',1,1,'suggested','model','2','business_rule','statement')`); err != nil {
		t.Fatal(err)
	}
	gen := &generatorStub{profile: "m"}
	req := Request{ProjectID: "p", RequirementID: "r", TextVersion: 1, Revision: 1, SuggestionRevision: 1, Target: Target{Semantic: "constraint", Representation: "test_case", Locale: "ru"}}
	service := New(db, gen)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen.after = func() {
		r, status, _, err := service.Explain(ctx, "a", req)
		if err != nil || r != nil || status != "busy" {
			t.Fatalf("competing call: %+v %s %v", r, status, err)
		}
		cancel()
	}
	r, status, _, err := service.Explain(ctx, "a", req)
	if err != nil || r != nil || status != "unavailable" {
		t.Fatalf("canceled: %+v %s %v", r, status, err)
	}
	gen.after = nil
	r, status, _, err = service.Explain(context.Background(), "a", req)
	if err != nil || r == nil || status != "ready" || gen.calls != 2 {
		t.Fatalf("after cancellation: %+v %s %v, calls=%d", r, status, err, gen.calls)
	}
	if _, err := db.Exec(`UPDATE requirements SET archived_at=clock_timestamp() WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	r, status, _, err = service.Explain(context.Background(), "a", req)
	if err != nil || r != nil || status != "stale" || gen.calls != 2 {
		t.Fatalf("after archive: %+v %s %v, calls=%d", r, status, err, gen.calls)
	}
}
