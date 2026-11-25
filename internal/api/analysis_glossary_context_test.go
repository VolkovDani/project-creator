package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type contextComparison struct {
	mu      sync.Mutex
	calls   []string
	entered chan struct{}
	release chan struct{}
}

func (g *contextComparison) Profile() string { return "pair-context-fixture" }

func (g *contextComparison) Compare(ctx context.Context, p analysis.Pair) (analysis.Finding, error) {
	payload, _ := json.Marshal(p)
	g.mu.Lock()
	g.calls = append(g.calls, string(payload))
	g.mu.Unlock()
	if g.entered != nil {
		g.entered <- struct{}{}
		select {
		case <-g.release:
		case <-ctx.Done():
			return analysis.Finding{}, ctx.Err()
		}
	}
	return analysis.Finding{LeftID: p.Left.ID, RightID: p.Right.ID, LeftVersion: p.Left.Version, RightVersion: p.Right.Version,
		Verdict: "compatible", Explanation: "Правила могут выполняться одновременно."}, nil
}

func (g *contextComparison) inputs() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.calls...)
}

func analysisGlossaryFixture(t *testing.T, generator *contextComparison) func(string) map[string]any {
	t.Helper()
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = s.db.Exec(`
	 INSERT INTO projects VALUES ('p','a','P','G','','now','now'),('q','b','Q','G','','now','now');
	 INSERT INTO requirements VALUES
	 ('a','a','p','ГП подписывает протокол.',1,'now','now'),
	 ('b','a','p','Заказчик отправляет результат.',1,'now','now'),
	 ('c','a','p','Срок составляет пять дней.',1,'now','now');
	 INSERT INTO glossary_terms(workspace,project_id,id,name,definition,meaning_context,variants,version,source_id,source_version)
	 VALUES('b','q','foreign','ГП','Чужое определение.','','[]',1,'',0);
	`); err != nil {
		t.Fatal(err)
	}
	return func(q string) map[string]any {
		t.Helper()
		out := summaryRequest(t, s, "token-a", q)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out
	}
}

func savePairGlossaryFixture(t *testing.T, request func(string) map[string]any, id string, version int, name, definition, variant string) string {
	t.Helper()
	edit := ""
	if id != "" {
		edit = fmt.Sprintf("id:%q version:%d ", id, version)
	}
	variants := "[]"
	if variant != "" {
		variants = fmt.Sprintf("[{kind:\"abbreviation\" text:%q}]", variant)
	}
	q := fmt.Sprintf(`mutation{saveGlossaryTerm(input:{projectId:"p" %sname:%q definition:%q context:"" variants:%s}){term{id} fieldErrors{code}}}`, edit, name, definition, variants)
	value := request(q)["data"].(map[string]any)["saveGlossaryTerm"].(map[string]any)
	if value["term"] == nil {
		t.Fatal(value)
	}
	return value["term"].(map[string]any)["id"].(string)
}

func TestPairGlossaryChangeKeepsUnrelatedComparisonsReusable(t *testing.T) {
	g := &contextComparison{}
	request := analysisGlossaryFixture(t, g)
	const start = `mutation{startProjectAnalysis(projectId:"p",mode:"all"){id}}`
	const staleQuery = `{projectAnalysisSummary(projectId:"p"){stale}}`
	isStale := func() bool {
		return request(staleQuery)["data"].(map[string]any)["projectAnalysisSummary"].(map[string]any)["stale"].(bool)
	}
	run := func(reused, total int) map[string]any {
		t.Helper()
		request(start)
		result := waitAnalysis(t, request, "p")
		if result["status"] != "completed" || result["reusedPairs"] != float64(reused) || len(g.inputs()) != total {
			t.Fatalf("unexpected completion: reused=%v, calls=%d, status=%v", result["reusedPairs"], len(g.inputs()), result["status"])
		}
		return result
	}
	run(0, 3)
	savePairGlossaryFixture(t, request, "", 0, "План", "График.", "")
	if isStale() {
		t.Fatal("unrelated term marked results stale")
	}
	run(3, 3)
	term := savePairGlossaryFixture(t, request, "", 0, "Генеральный подрядчик", "Отвечает за работы.", "ГП")
	if !isStale() {
		t.Fatal("new applicable definition did not mark comparisons stale")
	}
	result := run(1, 5)
	for _, raw := range g.inputs()[3:] {
		if !strings.Contains(raw, `"confirmedGlossary":[`) || !strings.Contains(raw, "Отвечает за работы.") || !strings.Contains(raw, `"version":1,"spelling":"ГП"`) || strings.Contains(raw, "Чужое определение") {
			t.Fatalf("unscoped or missing glossary context: %s", raw)
		}
	}
	savePairGlossaryFixture(t, request, term, 1, "Генеральный подрядчик", "Проверяет работы.", "ГП")
	if !isStale() {
		t.Fatal("changed definition did not mark comparisons stale")
	}
	for _, item := range result["pairs"].([]any) {
		p := item.(map[string]any)
		q := fmt.Sprintf(`{analysisPair(projectId:"p",analysisId:%q,pairId:%q){glossaryStale glossaryContext{termId version definition}}}`, result["id"], p["id"])
		d := request(q)["data"].(map[string]any)["analysisPair"].(map[string]any)
		hints := d["glossaryContext"].([]any)
		affected := p["left"].(map[string]any)["id"] == "a"
		if d["glossaryStale"] != affected || affected && (len(hints) != 1 || hints[0].(map[string]any)["termId"] != term || hints[0].(map[string]any)["definition"] != "Отвечает за работы.") || !affected && len(hints) != 0 {
			t.Fatal(d)
		}
	}
	run(1, 7)
	request(fmt.Sprintf(`mutation{deleteGlossaryTerm(projectId:"p",id:%q,version:2){deleted}}`, term))
	if !isStale() {
		t.Fatal("removed definition did not mark comparisons stale")
	}
	run(3, 7)
}

func TestPairGlossaryEditFencesInFlightComparison(t *testing.T) {
	g := &contextComparison{entered: make(chan struct{}, 1), release: make(chan struct{})}
	request := analysisGlossaryFixture(t, g)
	term := savePairGlossaryFixture(t, request, "", 0, "Генеральный подрядчик", "Отвечает за работы.", "ГП")
	request(`mutation{startProjectAnalysis(projectId:"p",mode:"all"){id}}`)
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("comparison did not start")
	}
	savePairGlossaryFixture(t, request, term, 1, "Генеральный подрядчик", "Проверяет работы.", "ГП")
	close(g.release)
	result := waitAnalysis(t, request, "p")
	if result["status"] != "failed" || result["errorCode"] != "GLOSSARY_CHANGED" || result["completedPairs"] != float64(0) || len(g.inputs()) != 1 {
		t.Fatalf("in-flight stale comparison returned: %v", result)
	}
}
