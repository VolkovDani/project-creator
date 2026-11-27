package explanation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
)

type generatorStub struct {
	calls     int
	profile   string
	source    Source
	after     func()
	err       error
	failAxis  Axis
	requested []Axis
	deadline  time.Time
}

func (g *generatorStub) ExplanationProfile() string { return g.profile }
func (g *generatorStub) ExplainClassificationAxes(ctx context.Context, source Source, target Target, axes []Axis) (map[Axis]AxisResult, error) {
	g.calls++
	g.source = source
	g.requested = slices.Clone(axes)
	deadline, ok := ctx.Deadline()
	if !ok {
		panic("missing inference deadline")
	}
	g.deadline = deadline
	if g.after != nil {
		g.after()
	}
	results := make(map[Axis]AxisResult, len(axes))
	for _, axis := range axes {
		if g.err != nil && (g.failAxis == "" || g.failAxis == axis) {
			return results, g.err
		}
		results[axis] = AxisResult{Assessment: Assessment{Verdict: "partial"}, Questions: []string{"Original question."}, Limitations: target.Locale}
	}
	return results, nil
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

func TestAdmissionAndWorkspaceWindows(t *testing.T) {
	service := New(nil, nil)
	now := time.Unix(100, 0)
	check := func(workspace string, at time.Time, want string) {
		t.Helper()
		if status := service.admitLocked(workspace, at); status != want {
			t.Fatalf("status %q, want %q", status, want)
		}
	}
	check("a", now, "")
	check("a", now, "busy")
	service.release()
	for i := 1; i < callsPerMinute; i++ {
		check("a", now, "")
		service.release()
	}
	check("a", now, "busy")
	check("b", now, "")
	service.release()
	check("a", now.Add(time.Minute), "")
	service.release()
	if _, exists := service.recent["b"]; exists {
		t.Fatal("expired workspace retained")
	}
}

func TestCachedAxesExpireOnlyWithAConfiguredLifetime(t *testing.T) {
	db, req := classifiedRequirement(t)
	gen := &generatorStub{profile: "m"}
	service := New(db, gen)
	check := func(s *Service, wantCalls int) *Result {
		t.Helper()
		result, status, _, err := s.Explain(t.Context(), "a", req)
		if err != nil || status != "ready" || result == nil || gen.calls != wantCalls {
			t.Fatalf("result=%+v status=%q calls=%d error=%v", result, status, gen.calls, err)
		}
		return result
	}
	check(service, 1).Questions[0] = "modified"
	if _, err := db.Exec("UPDATE classification_explanation_cache SET created_at='2000-01-01' WHERE axis='meaning'"); err != nil {
		t.Fatal(err)
	}
	// Full hits remain available while the inference slot is occupied.
	if status := service.admitLocked("other", time.Now()); status != "" {
		t.Fatal(status)
	}
	if check(service, 1).Questions[0] != "Original question." {
		t.Fatal("mutating a returned result changed the cache")
	}
	service.release()
	expiring := NewWithCacheTTL(db, gen, 24*time.Hour)
	check(expiring, 2)
	if !slices.Equal(gen.requested, []Axis{MeaningAxis}) {
		t.Fatalf("only the expired meaning should regenerate, got %v", gen.requested)
	}
	check(expiring, 2)
}

func TestSuccessfulAxisRemainsCachedAfterOtherAxisFails(t *testing.T) {
	db, req := classifiedRequirement(t)
	gen := &generatorStub{profile: "m", err: ErrInvalidOutput, failAxis: FormAxis}
	result, status, _, err := New(db, gen).Explain(t.Context(), "a", req)
	if err != nil || result != nil || status != "invalid_output" {
		t.Fatalf("failed form: %+v %s %v", result, status, err)
	}
	gen.err = nil
	result, status, _, err = New(db, gen).Explain(t.Context(), "a", req)
	if err != nil || result == nil || status != "ready" || !slices.Equal(gen.requested, []Axis{FormAxis}) {
		t.Fatalf("retry: %+v %s %v (axes %v)", result, status, err, gen.requested)
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

func classifiedRequirement(t *testing.T) (*sql.DB, Request) {
	t.Helper()
	db := database(t)
	if _, err := db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');
 INSERT INTO requirements VALUES('r','a','p','source',1,'now','now');
 INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type)
 VALUES('a','p','r',1,1,'suggested','model','2','business_rule','statement')`); err != nil {
		t.Fatal(err)
	}
	return db, Request{ProjectID: "p", RequirementID: "r", TextVersion: 1, Revision: 1, SuggestionRevision: 1, Target: Target{Semantic: "constraint", Representation: "test_case", Locale: "ru"}}
}

func TestGenerationFailureStatusAndRetry(t *testing.T) {
	db, req := classifiedRequirement(t)
	for _, tc := range []struct {
		name, status string
		err          error
	}{
		{"invalid output", "invalid_output", ErrInvalidOutput},
		{"input too long", "input_too_long", ErrInputTooLong},
		{"provider failure", "unavailable", errors.New("private provider details")},
		{"cancelled", "cancelled", context.Canceled},
		{"deadline exceeded", "timeout", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gen := &generatorStub{profile: tc.name, err: fmt.Errorf("private inference details: %w", tc.err)}
			service := New(db, gen)
			result, status, fields, err := service.Explain(context.Background(), "a", req)
			if err != nil || len(fields) != 0 || result != nil || status != tc.status || gen.calls != 1 {
				t.Fatalf("failed call: result=%+v status=%q fields=%+v err=%v calls=%d", result, status, fields, err, gen.calls)
			}
			gen.err = nil
			result, status, _, err = service.Explain(context.Background(), "a", req)
			if err != nil || result == nil || status != "ready" || gen.calls != 2 {
				t.Fatalf("explicit retry: result=%+v status=%q err=%v calls=%d", result, status, err, gen.calls)
			}
		})
	}
}

func TestExpiredExplanationRequest(t *testing.T) {
	db, req := classifiedRequirement(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()
	gen := &generatorStub{profile: "m"}
	result, status, fields, err := New(db, gen).Explain(ctx, "a", req)
	if err != nil || len(fields) != 0 || result != nil || status != "timeout" || gen.calls != 0 {
		t.Fatalf("expired request: result=%+v status=%q fields=%+v err=%v calls=%d", result, status, fields, err, gen.calls)
	}
}

func TestExplanationBudgetCoversBothSequentialModelCalls(t *testing.T) {
	db, req := classifiedRequirement(t)
	gen := &generatorStub{profile: "m"}
	start := time.Now()
	result, status, _, err := New(db, gen).Explain(context.Background(), "a", req)
	if err != nil || status != "ready" || result == nil {
		t.Fatalf("assessment failed: %q %v", status, err)
	}
	const budget = 180 * time.Second
	if gen.deadline.Before(start.Add(budget)) || gen.deadline.After(time.Now().Add(budget)) {
		t.Fatalf("assessment deadline %s after start must cover both model calls", gen.deadline.Sub(start))
	}
}

func TestExplanationRequiresConfiguredGenerator(t *testing.T) {
	db, req := classifiedRequirement(t)
	for _, generator := range []Generator{nil, &generatorStub{}} {
		result, status, fields, err := New(db, generator).Explain(context.Background(), "a", req)
		if err != nil || len(fields) != 0 || result != nil || status != "unavailable" {
			t.Fatalf("missing configuration: result=%+v status=%q fields=%+v err=%v", result, status, fields, err)
		}
	}
}

func TestCanceledGenerationIsReleasedAndNeverCached(t *testing.T) {
	db, req := classifiedRequirement(t)
	gen := &generatorStub{profile: "m"}
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
	if err != nil || r != nil || status != "cancelled" {
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
