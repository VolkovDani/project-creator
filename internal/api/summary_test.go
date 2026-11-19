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
	batch := summaryRequest(t, s, "token-a", fmt.Sprintf(`{requirementSummariesByIds(projectId:%q,requirementIds:[%q,%q]){requirementId status}}`, project, id1, id2))
	if batch["errors"] != nil || len(batch["data"].(map[string]any)["requirementSummariesByIds"].([]any)) != 2 {
		t.Fatalf("batch response: %v", batch)
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
