package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type replacementAPIGenerator struct{ entered, release chan struct{} }

func (g *replacementAPIGenerator) Profile() string { return "replacement-api" }
func (g *replacementAPIGenerator) Compare(ctx context.Context, p analysis.Pair) (analysis.Finding, error) {
	select {
	case g.entered <- struct{}{}:
	default:
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return analysis.Finding{}, ctx.Err()
	}
	return analysis.Finding{LeftID: p.Left.ID, RightID: p.Right.ID, LeftVersion: p.Left.Version, RightVersion: p.Right.Version, Verdict: "compatible", Explanation: "Compatible fixture"}, nil
}

func TestGraphQLReplacementIsScopedValidatedAndDurable(t *testing.T) {
	g := &replacementAPIGenerator{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s, e := NewConfigured(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, Config{Generator: g, DefaultAnalysisThreshold: 0.78})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	defer close(g.release)
	if _, e = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('r1','a','p','First requirement',1,'now','now'),('r2','a','p','Second requirement',1,'now','now')`); e != nil {
		t.Fatal(e)
	}
	request := func(token, q string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": q})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	if got := request("token-a", `{analysisDefaultThreshold}`); got["errors"] != nil || got["data"].(map[string]any)["analysisDefaultThreshold"] != 0.78 {
		t.Fatalf("effective default: %v", got)
	}
	start := request("token-a", `mutation{startProjectAnalysis(projectId:"p",mode:"all"){id similarityThreshold}}`)
	if start["errors"] != nil {
		t.Fatal(start)
	}
	started := start["data"].(map[string]any)["startProjectAnalysis"].(map[string]any)
	if started["similarityThreshold"] != 0.78 {
		t.Fatalf("omitted start threshold: %v", start)
	}
	id := started["id"].(string)
	select {
	case <-g.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("model did not start")
	}
	mutation := `mutation{replaceProjectAnalysis(projectId:"p",analysisId:"` + id + `",mode:"all",strategy:"wait"){id stage status}}`
	if denied := request("token-b", mutation); denied["errors"] == nil {
		t.Fatal("foreign replacement accepted")
	}
	invalid := request("token-a", `mutation{replaceProjectAnalysis(projectId:"p",analysisId:"`+id+`",mode:"invalid",threshold:2,strategy:"wait"){id}}`)
	if invalid["errors"] == nil {
		t.Fatal("invalid options accepted")
	}
	// Explicit boundary values must survive even with a nonzero deployment default.
	for _, threshold := range []float64{0, 1} {
		out := request("token-a", fmt.Sprintf(`mutation{replaceProjectAnalysis(projectId:"p",analysisId:%q,mode:"all",threshold:%g,strategy:"wait"){id}}`, id, threshold))
		if out["errors"] != nil {
			t.Fatal(out)
		}
		var queued float64
		if err := s.db.QueryRow(`SELECT threshold FROM analysis_replacements WHERE run_id=$1`, id).Scan(&queued); err != nil || queued != threshold {
			t.Fatalf("explicit replacement threshold: got %v, %v; want %v", queued, err, threshold)
		}
	}
	for i := 0; i < 2; i++ {
		out := request("token-a", mutation)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		run := out["data"].(map[string]any)["replaceProjectAnalysis"].(map[string]any)
		if run["id"] != id || run["stage"] != "waiting_current_pair" || run["status"] != "running" {
			t.Fatal(out)
		}
	}
	var requests int
	if e = s.db.QueryRow(`SELECT count(*) FROM analysis_replacements WHERE run_id=$1 AND successor_id IS NULL AND threshold=0.78`, id).Scan(&requests); e != nil || requests != 1 {
		t.Fatalf("durable request %d %v", requests, e)
	}
}
