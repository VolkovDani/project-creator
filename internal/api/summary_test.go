package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type apiSummaryGenerator struct {
	calls   atomic.Int32
	text    func(analysis.Source) string
	first   chan struct{}
	release chan struct{}
}

func (g *apiSummaryGenerator) Profile() string { return "api-summary-v1" }
func (g *apiSummaryGenerator) Provenance() string {
	return `{"model":"deterministic-test","revision":"1"}`
}
func (g *apiSummaryGenerator) Summarize(ctx context.Context, source analysis.Source) (string, error) {
	n := g.calls.Add(1)
	if n == 1 && g.first != nil {
		close(g.first)
	}
	if n == 1 && g.release != nil {
		<-g.release // Deliberately ignores cancellation to exercise late-result fencing.
	}
	return g.text(source), nil
}

type apiSummaryFinalGenerator struct {
	calls atomic.Int32
	bad   atomic.Bool
}

func (*apiSummaryFinalGenerator) Profile() string { return "api-full-source-v1" }
func (g *apiSummaryFinalGenerator) Compare(_ context.Context, pair analysis.Pair) (analysis.Finding, error) {
	g.calls.Add(1)
	if pair.Left.Text != strings.Repeat("full left obligation ", 120) || pair.Right.Text != strings.Repeat("full right obligation ", 120) || pair.Preclassification != nil {
		g.bad.Store(true)
	}
	return analysis.Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "compatible", Explanation: "Compared full requirement sources."}, nil
}

func summaryRequest(t *testing.T, s *Server, token, query string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var out map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode GraphQL response %q: %v", w.Body.String(), err)
	}
	return out
}

func waitAPISummary(t *testing.T, s *Server, token, project, id, status string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	query := fmt.Sprintf(`{requirementSummariesByIds(projectId:%q,requirementIds:[%q]){requirementId projectId version status text provenance errorCode}}`, project, id)
	for time.Now().Before(deadline) {
		out := summaryRequest(t, s, token, query)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		rows := out["data"].(map[string]any)["requirementSummariesByIds"].([]any)
		if len(rows) == 1 && rows[0].(map[string]any)["status"] == status {
			return rows[0].(map[string]any)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("summary %s did not reach %s", id, status)
	return nil
}

func TestGraphQLSummaryCRUDBatchAuthorizationAndCommittedInvalidation(t *testing.T) {
	g := &apiSummaryGenerator{text: func(source analysis.Source) string { return "derived:" + source.ID }}
	s, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	create := summaryRequest(t, s, "token-a", `mutation{createProject(input:{name:"Summary API",goal:"G",description:""}){project{id}}}`)
	if create["errors"] != nil {
		t.Fatal(create)
	}
	project := create["data"].(map[string]any)["createProject"].(map[string]any)["project"].(map[string]any)["id"].(string)
	createReq := func(text string) string {
		t.Helper()
		q := fmt.Sprintf(`mutation{createRequirement(input:{projectId:%q,text:%q}){requirement{id version}}}`, project, text)
		out := summaryRequest(t, s, "token-a", q)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)["createRequirement"].(map[string]any)["requirement"].(map[string]any)["id"].(string)
	}
	id1 := createReq("first full source")
	id2 := createReq("second full source")
	first := waitAPISummary(t, s, "token-a", project, id1, "ready")
	second := waitAPISummary(t, s, "token-a", project, id2, "ready")
	if first["version"] != float64(1) || first["text"] != "derived:"+id1 || first["provenance"] == "" || second["text"] != "derived:"+id2 {
		t.Fatalf("summary states: %v / %v", first, second)
	}
	saved := summaryRequest(t, s, "token-a", fmt.Sprintf(`mutation{saveGlossaryTerm(input:{projectId:%q,name:"derived",definition:"A preview",context:"",variants:[]}){term{id} fieldErrors{code}}}`, project))
	if saved["errors"] != nil {
		t.Fatal(saved)
	}
	batch := summaryRequest(t, s, "token-a", fmt.Sprintf(`{requirementSummariesByIds(projectId:%q,requirementIds:[%q,%q]){requirementId status glossaryMatches {start end text terms {definition}}}}`, project, id1, id2))
	if batch["errors"] != nil || len(batch["data"].(map[string]any)["requirementSummariesByIds"].([]any)) != 2 {
		t.Fatalf("batch response: %v", batch)
	}
	for _, row := range batch["data"].(map[string]any)["requirementSummariesByIds"].([]any) {
		matches := row.(map[string]any)["glossaryMatches"].([]any)
		if len(matches) != 1 {
			t.Fatalf("summary matches: %v", matches)
		}
		m := matches[0].(map[string]any)
		if m["start"] != float64(0) || m["end"] != float64(7) || m["text"] != "derived" || m["terms"].([]any)[0].(map[string]any)["definition"] != "A preview" {
			t.Fatal(m)
		}
	}
	retryReady := summaryRequest(t, s, "token-a", fmt.Sprintf(`mutation{retryRequirementSummary(projectId:%q,requirementId:%q){status glossaryMatches{text}}}`, project, id1))
	ready := retryReady["data"].(map[string]any)["retryRequirementSummary"].(map[string]any)
	if ready["status"] != "ready" || len(ready["glossaryMatches"].([]any)) != 1 {
		t.Fatalf("ready mutation: %v", ready)
	}
	if foreign := summaryRequest(t, s, "token-b", fmt.Sprintf(`{requirementSummariesByIds(projectId:%q,requirementIds:[%q]){text}}`, project, id1)); foreign["errors"] == nil {
		t.Fatalf("foreign summary read succeeded: %v", foreign)
	}
	var before int
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys @> ARRAY[$1]`, "project:"+project+":summaries").Scan(&before); err != nil {
		t.Fatal(err)
	}
	update := summaryRequest(t, s, "token-a", fmt.Sprintf(`mutation{updateRequirement(input:{id:%q,projectId:%q,version:1,text:"updated full source"}){requirement{version}}}`, id1, project))
	if update["errors"] != nil {
		t.Fatal(update)
	}
	updated := waitAPISummary(t, s, "token-a", project, id1, "ready")
	if updated["version"] != float64(2) {
		t.Fatalf("updated summary: %v", updated)
	}
	var after, committedVersion int
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys @> ARRAY[$1]`, "project:"+project+":summaries").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT version FROM requirements WHERE workspace='a' AND project_id=$1 AND id=$2`, project, id1).Scan(&committedVersion); err != nil {
		t.Fatal(err)
	}
	if committedVersion != 2 || after <= before {
		t.Fatalf("summary invalidation not committed with CRUD: version=%d events %d -> %d", committedVersion, before, after)
	}
}

func TestGraphQLSummaryCancelRetryAndFullSourcePipeline(t *testing.T) {
	blocked := make(chan struct{})
	g := &apiSummaryGenerator{text: func(source analysis.Source) string { return strings.Repeat("derived ", 60) + source.ID }, first: make(chan struct{}), release: blocked}
	final := &apiSummaryFinalGenerator{}
	s, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, final, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO requirements VALUES('left','a','p',$1,1,'now','now'),('right','a','p',$2,1,'now','now')`, strings.Repeat("full left obligation ", 120), strings.Repeat("full right obligation ", 120)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-g.first:
	case <-time.After(5 * time.Second):
		t.Fatal("automatic summary generation did not begin")
	}
	cancel := summaryRequest(t, s, "token-a", `mutation{cancelRequirementSummary(projectId:"p",requirementId:"left"){status}}`)
	if cancel["errors"] != nil || cancel["data"].(map[string]any)["cancelRequirementSummary"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("cancel response: %v", cancel)
	}
	close(blocked)
	waitAPISummary(t, s, "token-a", "p", "left", "cancelled")
	right := waitAPISummary(t, s, "token-a", "p", "right", "ready")
	if len(right["text"].(string)) <= 350 {
		t.Fatal("expected a stored preview over the former admission limit")
	}
	start := summaryRequest(t, s, "token-a", `mutation{startProjectAnalysis(projectId:"p",mode:"all"){id status}}`)
	if start["errors"] != nil {
		t.Fatal(start)
	}
	analysisID := start["data"].(map[string]any)["startProjectAnalysis"].(map[string]any)["id"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := s.analysis.Get(context.Background(), "a", "p", analysisID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current.Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, err := s.analysis.Get(context.Background(), "a", "p", analysisID)
	if err != nil || current.Status != "completed" || current.ClassifierProfile != "" || current.LayaCompletedPairs != 0 || final.calls.Load() != 1 || final.bad.Load() {
		t.Fatalf("pipeline status=%v final=%d bad=%v err=%v", current, final.calls.Load(), final.bad.Load(), err)
	}
	retry := summaryRequest(t, s, "token-a", `mutation{retryRequirementSummary(projectId:"p",requirementId:"left"){status}}`)
	if retry["errors"] != nil {
		t.Fatal(retry)
	}
	waitAPISummary(t, s, "token-a", "p", "left", "ready")
}

func TestGraphQLRetryFailedRequirementSummaries(t *testing.T) {
	g := &apiSummaryGenerator{text: func(analysis.Source) string { return "Derived" }}
	s, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Keep every queue state stable while two clients submit the same retry.
	s.summaries.Close()
	_, err = s.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('other','a','P','G','','now','now'), ('private','b','P','G','','now','now');
		INSERT INTO requirements SELECT 'retry-' || i,'a','p','Source',2,'now','now' FROM generate_series(1,101) AS i;
		INSERT INTO requirements SELECT state,'a','p','Source',1,'now','now'
		  FROM unnest(ARRAY['ready','queued','running','cancelled','stale','missing']) AS state;
		INSERT INTO requirements VALUES ('other-r','a','other','Source',1,'now','now'), ('private-r','b','private','Source',1,'now','now');
		UPDATE requirement_summaries SET status='failed', error_code=CASE WHEN requirement_id='retry-1' THEN 'INTERRUPTED' ELSE 'GENERATION_FAILED' END,
		  attempt='old-attempt',text='discard' WHERE requirement_id LIKE 'retry-%' OR project_id <> 'p';
		UPDATE requirement_summaries SET status=requirement_id, text=CASE WHEN requirement_id='ready' THEN 'Derived' ELSE '' END
		  WHERE requirement_id IN ('ready','queued','running','cancelled','stale');
		DELETE FROM requirement_summaries WHERE requirement_id='missing';
		INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status)
		  VALUES ('a','p','retry-1',1,'api-summary-v1','historical','failed'), ('a','p','retry-1',2,'old-profile','historical','failed');
	`)
	if err != nil {
		t.Fatal(err)
	}
	const count = `{amount:failedRequirementSummaryCount(projectId:"p")}`
	const retry = `mutation{amount:retryFailedRequirementSummaries(projectId:"p")}`
	amount := func(query string) int {
		t.Helper()
		out := summaryRequest(t, s, "token-a", query)
		if out["errors"] != nil {
			t.Fatalf("operation returned errors: %v", out)
		}
		return int(out["data"].(map[string]any)["amount"].(float64))
	}
	if got := amount(count); got != 101 {
		t.Fatalf("project-wide failure count = %d, want 101", got)
	}
	for _, q := range []string{count, retry} {
		if out := summaryRequest(t, s, "token-b", q); out["errors"] == nil {
			t.Fatalf("foreign project access succeeded: %v", out)
		}
	}

	var seq int64
	if err := s.db.QueryRow(`SELECT MAX(seq) FROM cache_events`).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	responses := make(chan map[string]any, 1)
	go func() {
		defer close(responses)
		responses <- summaryRequest(t, s, "token-a", retry)
	}()
	counts := make(map[float64]int)
	for _, out := range []map[string]any{summaryRequest(t, s, "token-a", retry), <-responses} {
		if out["errors"] != nil {
			t.Fatal(out)
		}
		counts[out["data"].(map[string]any)["amount"].(float64)]++
	}
	if counts[0] != 1 || counts[101] != 1 || amount(count) != 0 {
		t.Fatalf("concurrent retries: %v", counts)
	}
	for _, check := range []struct {
		query string
		want  int
	}{
		{`SELECT count(*) FROM requirement_summaries WHERE requirement_id LIKE 'retry-%' AND version=2 AND profile='api-summary-v1' AND status='queued' AND error_code='' AND attempt='' AND text=''`, 101},
		{`SELECT count(*) FROM requirement_summaries WHERE (version=1 OR profile='old-profile') AND status='failed'`, 4},
		{`SELECT count(*) FROM requirement_summaries WHERE requirement_id=status`, 5},
		{`SELECT count(*) FROM requirement_summaries WHERE requirement_id='missing'`, 0},
	} {
		var got int
		if err := s.db.QueryRow(check.query).Scan(&got); err != nil || got != check.want {
			t.Errorf("%s: got %d, want %d, err %v", check.query, got, check.want, err)
		}
	}
	var events int
	if err := s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE seq>$1 AND workspace='a' AND keys=ARRAY['project:p:summaries']`, seq).Scan(&events); err != nil || events == 0 {
		t.Fatalf("committed project summary invalidation: events %d, err %v", events, err)
	}

	// A newer instance can change the active profile while this server is still reachable.
	if _, err := s.db.Exec(`UPDATE summary_profiles SET profile='old-profile' WHERE workspace='a';
		UPDATE requirement_summaries SET status='failed' WHERE requirement_id='retry-1' AND version=2`); err != nil {
		t.Fatal(err)
	}
	if amount(count) != 0 || amount(retry) != 0 {
		t.Fatal("superseded service profile can still count or retry summaries")
	}
}

func TestGraphQLFailedSummariesWithoutGenerator(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES ('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	out := summaryRequest(t, s, "token-a", `{failedRequirementSummaryCount(projectId:"p")}`)
	if out["errors"] != nil || out["data"].(map[string]any)["failedRequirementSummaryCount"] != float64(0) {
		t.Fatalf("count without a generator: %v", out)
	}
	out = summaryRequest(t, s, "token-a", `mutation{retryFailedRequirementSummaries(projectId:"p")}`)
	if out["errors"] == nil {
		t.Fatalf("retry without a generator succeeded: %v", out)
	}
}
