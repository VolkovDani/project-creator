package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/glossary"
)

type definitionStub struct {
	apiSummaryGenerator
	calls       []glossary.DefinitionRequest
	definition  string
	err         error
	beforeReply func()
}

func (s *definitionStub) SuggestDefinition(_ context.Context, r glossary.DefinitionRequest) (string, error) {
	s.calls = append(s.calls, r)
	if s.beforeReply != nil {
		s.beforeReply()
	}
	return s.definition, s.err
}

func TestSuggestGlossaryDefinitionScopedDraft(t *testing.T) {
	g := &definitionStub{definition: "Подпись для проверки отчёта."}
	dsn := testDatabase(t)
	tokens := map[string]string{"token-a": "a", "token-b": "b"}
	s, err := NewWithSummaryProviders(dsn, tokens, nil, nil, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const source = "😀 ЭЦП нужна для проверки отчёта."
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','a','Q','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO requirements VALUES('r','a','p',$1,2,'now','now')`, source); err != nil {
		t.Fatal(err)
	}

	request := func(token, project, sourceID, name string, version, start, end int) map[string]any {
		t.Helper()
		query := fmt.Sprintf(`mutation{suggestGlossaryDefinition(input:{projectId:%q,sourceId:%q,sourceVersion:%d,start:%d,end:%d,name:%q}){definition status fieldErrors{path code messageKey params{key value}}}}`, project, sourceID, version, start, end, name)
		out := summaryRequest(t, s, token, query)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)["suggestGlossaryDefinition"].(map[string]any)
	}
	for _, tc := range []struct {
		token, project, sourceID string
		version, start, end      int
	}{
		{"token-b", "p", "r", 2, 2, 5}, {"token-a", "q", "r", 2, 2, 5}, {"token-a", "p", "absent", 2, 2, 5},
		{"token-a", "p", "r", 1, 2, 5}, {"token-a", "p", "r", 2, 3, 6}, {"token-a", "p", "r", 2, -1, 5},
		{"token-a", "p", "r", 2, 2, 50}, {"token-a", "p", "r", 2, 5, 2},
	} {
		got := request(tc.token, tc.project, tc.sourceID, "", tc.version, tc.start, tc.end)
		if got["status"] != "stale" || got["definition"] != nil || len(g.calls) != 0 {
			t.Fatalf("unexpected suggestion for %+v: %v, %d calls", tc, got, len(g.calls))
		}
	}
	got := request("token-a", "p", "r", strings.Repeat("Я", contracts.GlossaryRules().NameMax+1), 2, 2, 5)
	if got["status"] != "invalid" || len(g.calls) != 0 {
		t.Fatal(got)
	}
	field := got["fieldErrors"].([]any)[0].(map[string]any)
	if field["code"] != "MAX_LENGTH" || field["messageKey"] != "validation.maxLength" || field["path"].([]any)[1] != "name" {
		t.Fatal(field)
	}

	got = request("token-a", "p", "r", " Название ", 2, 2, 5)
	want := glossary.DefinitionRequest{Text: source, Abbreviation: "ЭЦП", Name: "Название", Start: 2, End: 5}
	if got["status"] != "suggested" || got["definition"] != g.definition || len(g.calls) != 1 || g.calls[0] != want {
		t.Fatalf("%v, requests=%+v", got, g.calls)
	}
	var saved int
	if err := s.db.QueryRow(`SELECT count(*) FROM glossary_terms`).Scan(&saved); err != nil || saved != 0 {
		t.Fatalf("saved terms=%d, err=%v", saved, err)
	}
	g.definition = ""
	got = request("token-a", "p", "r", "", 2, 2, 5)
	if got["status"] != "insufficient" || got["definition"] != nil || g.calls[1].Name != "" {
		t.Fatal(got)
	}
	g.err = errors.New("provider details")
	got = request("token-a", "p", "r", "", 2, 2, 5)
	if got["status"] != "unavailable" || got["definition"] != nil {
		t.Fatal(got)
	}

	g.err = nil
	g.definition = "Сторона договора."
	const phrase = "Генеральный подрядчик ("
	start := len([]rune(phrase))
	if _, err := s.db.Exec(`UPDATE requirements SET version=3, text=$1 WHERE id='r'`, phrase+"ГП) проверяет отчёт."); err != nil {
		t.Fatal(err)
	}
	got = request("token-a", "p", "r", "", 3, start, start+2)
	if got["status"] != "suggested" || g.calls[len(g.calls)-1].Name != "Генеральный подрядчик" {
		t.Fatal(got, g.calls)
	}

	g.beforeReply = func() {
		if _, err := s.db.Exec(`UPDATE requirements SET version=4, text='Совсем другое требование' WHERE id='r'`); err != nil {
			t.Fatal(err)
		}
	}
	got = request("token-a", "p", "r", "", 3, start, start+2)
	if got["status"] != "stale" || got["definition"] != nil {
		t.Fatal(got)
	}
}

func TestSuggestGlossaryDefinitionWithoutProvider(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','КЦ отвечает на звонок',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	out := summaryRequest(t, s, "token-a", `mutation{suggestGlossaryDefinition(input:{projectId:"p",sourceId:"r",sourceVersion:1,start:0,end:2,name:""}){status definition}}`)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	got := out["data"].(map[string]any)["suggestGlossaryDefinition"].(map[string]any)
	if got["status"] != "unavailable" || got["definition"] != nil {
		t.Fatal(got)
	}
}
