package api

import (
	"context"
	"fmt"
	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/summary"
	"strings"
	"testing"
)

func TestGlossaryCRUDIsolationAndCandidates(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','b','Q','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','Генеральный подрядчик (ГП) проверяет отчёт.',1,'now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	query := func(token, q, key string) any {
		t.Helper()
		out := summaryRequest(t, s, token, q)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)[key]
	}
	save := `mutation {saveGlossaryTerm(input:{projectId:"p",name:"Генеральный подрядчик",definition:"Исполнитель",context:"",variants:[{text:"ГП",kind:"abbreviation"}],sourceId:"r",sourceVersion:1}){term{id version} fieldErrors{code}}}`
	annotations := `{requirementGlossary(projectId:"p",requirementId:"r",version:1){matches{text} candidates{name abbreviation}}}`
	first := query("token-a", annotations, "requirementGlossary").(map[string]any)
	if len(first["candidates"].([]any)) != 1 || len(first["matches"].([]any)) != 0 {
		t.Fatal(first)
	}
	item := query("token-a", save, "saveGlossaryTerm").(map[string]any)
	if item["term"] == nil {
		t.Fatal(item)
	}
	id := item["term"].(map[string]any)["id"].(string)
	if query("token-b", save, "saveGlossaryTerm").(map[string]any)["term"] != nil {
		t.Fatal("cross-scope write")
	}
	if query("token-b", annotations, "requirementGlossary") != nil {
		t.Fatal("cross-scope read")
	}
	data := query("token-a", annotations, "requirementGlossary").(map[string]any)
	if len(data["matches"].([]any)) != 2 || len(data["candidates"].([]any)) != 0 {
		t.Fatal(data)
	}
	if got := query("token-a", `{glossaryTerms(projectId:"p",search:"гп",limit:10,offset:0){id}}`, "glossaryTerms").([]any); len(got) != 1 {
		t.Fatal(got)
	}
	// Find a second requirement by variant alone, irrespective of saved provenance.
	if _, err = s.db.Exec(`INSERT INTO requirements VALUES('s','a','p','ГП принимает отчёт',1,'now','now'),('x','a','p','АГП не совпадает',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	mentions := fmt.Sprintf(`{glossaryMentions(projectId:"p",termId:%q,first:20){items{id text} pageInfo{hasNextPage}}}`, id)
	for _, entry := range []struct {
		token string
		count int
	}{{"token-a", 2}, {"token-b", 0}} {
		page := query(entry.token, mentions, "glossaryMentions").(map[string]any)
		if len(page["items"].([]any)) != entry.count || page["pageInfo"].(map[string]any)["hasNextPage"] != false {
			t.Fatal(page)
		}
	}
	for _, version := range []int{2, 1} {
		v := query("token-a", fmt.Sprintf(`mutation {deleteGlossaryTerm(projectId:"p",id:%q,version:%d){deleted}}`, id, version), "deleteGlossaryTerm").(map[string]any)
		if v["deleted"] != (version == 1) {
			t.Fatal(v)
		}
	}
}

func TestProjectCandidatePagesOnlyContainScopedUnconfirmedEntries(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','Генеральный подрядчик (ГП)',1,'now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"token-a", "token-b"} {
		out := summaryRequest(t, s, token, `{glossaryCandidates(projectId:"p",limit:20,offset:0){hasMore items{name abbreviation sourceId sourceVersion}}}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		items := out["data"].(map[string]any)["glossaryCandidates"].(map[string]any)["items"].([]any)
		if (len(items) == 1) != (token == "token-a") {
			t.Fatal(out)
		}
	}
}

type glossaryGenerator struct{ apiSummaryGenerator }

func (g *glossaryGenerator) SummarizeWithGlossary(ctx context.Context, src analysis.Source, hints []glossary.Hint) (string, error) {
	if len(hints) != 1 {
		return "", fmt.Errorf("unexpected hints: %v", hints)
	}
	return hints[0].Name + " " + hints[0].Definition, nil
}

func TestGlossarySummaryRevisionsSurviveProviderDisableAndReactivation(t *testing.T) {
	dsn := testDatabase(t)
	tokens := map[string]string{"token-a": "a"}
	g := &glossaryGenerator{apiSummaryGenerator{text: func(analysis.Source) string { return "Обычный результат" }}}
	open := func(generator summary.Generator) *Server {
		t.Helper()
		s, e := NewWithSummaryProviders(dsn, tokens, nil, nil, generator)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	s := open(g)
	_, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','ГП проверяет',1,'now','now'),('u','a','p','Другая роль',1,'now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	waitAPISummary(t, s, "token-a", "p", "r", "ready")
	u := waitAPISummary(t, s, "token-a", "p", "u", "ready")
	q := `mutation {saveGlossaryTerm(input:{projectId:"p",name:"Генеральный подрядчик",definition:"Исполнитель",context:"",variants:[{text:"ГП",kind:"abbreviation"}]}){term{id version}}}`
	out := summaryRequest(t, s, "token-a", q)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	id := out["data"].(map[string]any)["saveGlossaryTerm"].(map[string]any)["term"].(map[string]any)["id"].(string)
	got := waitAPISummary(t, s, "token-a", "p", "r", "ready")
	if got["text"] != "Генеральный подрядчик Исполнитель" || !strings.Contains(got["provenance"].(string), id) {
		t.Fatal(got)
	}
	if fresh := waitAPISummary(t, s, "token-a", "p", "u", "ready"); fresh["createdAt"] != u["createdAt"] {
		t.Fatal("unrelated summary was regenerated")
	}
	s.Close()
	s = open(nil)
	q = fmt.Sprintf(`mutation {deleteGlossaryTerm(projectId:"p",id:%q,version:1){deleted}}`, id)
	if v := summaryRequest(t, s, "token-a", q); v["errors"] != nil {
		t.Fatal(v)
	}
	s.Close()
	s = open(g)
	got = waitAPISummary(t, s, "token-a", "p", "r", "ready")
	if got["text"] != "Обычный результат" {
		t.Fatalf("stale glossary context after restart: %v", got)
	}
}

func TestCandidateConnectionPaginatesMatchesInsideRequirement(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements SELECT 'a'||n,'a','p','empty',1,'now','now' FROM generate_series(1,50)n; INSERT INTO requirements VALUES('z','a','p','Alpha Beta (AB). Central Depot (CD).',1,'now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	read := func(token, suffix string) map[string]any {
		t.Helper()
		out := summaryRequest(t, s, token, `{glossaryCandidatesConnection(projectId:"p",first:1`+suffix+`){items{name} pageInfo{hasNextPage endCursor}}}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)["glossaryCandidatesConnection"].(map[string]any)
	}
	page := read("token-a", "")
	info := page["pageInfo"].(map[string]any)
	if len(page["items"].([]any)) != 1 || info["hasNextPage"] != true {
		t.Fatal(page)
	}
	next := read("token-a", fmt.Sprintf(",after:%q", info["endCursor"]))
	if next["items"].([]any)[0].(map[string]any)["name"] != "Central Depot" || next["pageInfo"].(map[string]any)["hasNextPage"] != false {
		t.Fatal(next)
	}
	if len(read("token-b", "")["items"].([]any)) != 0 {
		t.Fatal("foreign candidates exposed")
	}
}

func TestGlossaryUsesScopedAnalysisSourceVersion(t *testing.T) {
	generator := &cancelAPIGenerator{entered: make(chan struct{})}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','First obligation',1,'now','now'),('s','a','p','Second obligation',1,'now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	save := summaryRequest(t, s, "token-a", `mutation{saveGlossaryTerm(input:{projectId:"p",name:"First",definition:"Initial",context:"",variants:[]}){term{id}}}`)
	if save["errors"] != nil {
		t.Fatal(save)
	}
	run, err := s.analysis.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`UPDATE requirements SET text='Changed',version=2 WHERE id='r'`)
	if err != nil {
		t.Fatal(err)
	}
	query := fmt.Sprintf(`{requirementGlossary(projectId:"p",analysisId:%q,requirementId:"r",version:1){matches{text start end}}}`, run.ID)
	got := summaryRequest(t, s, "token-a", query)
	if got["errors"] != nil {
		t.Fatal(got)
	}
	matches := got["data"].(map[string]any)["requirementGlossary"].(map[string]any)["matches"].([]any)
	if len(matches) != 1 || matches[0].(map[string]any)["text"] != "First" {
		t.Fatal(got)
	}
	other := summaryRequest(t, s, "token-b", query)
	if other["errors"] != nil || other["data"].(map[string]any)["requirementGlossary"] != nil {
		t.Fatal(other)
	}
}
