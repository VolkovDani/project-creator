package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

func TestGlossaryChangeWhileArchivedRefreshesRestoredSummary(t *testing.T) {
	g := &glossaryGenerator{apiSummaryGenerator{text: func(analysis.Source) string { return "Обычный результат" }}}
	s, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a"}, nil, nil, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');
		INSERT INTO requirements VALUES('r','a','p','ГП проверяет отчёт',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	waitAPISummary(t, s, "token-a", "p", "r", "ready")
	run := func(query, key string) any {
		t.Helper()
		out := summaryRequest(t, s, "token-a", query)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)[key]
	}
	if run(`mutation {archiveRequirement(projectId:"p",id:"r",version:1)}`, "archiveRequirement") != true {
		t.Fatal("archive failed")
	}
	var stamp string
	if err := s.db.QueryRow(`SELECT archived_at FROM requirements WHERE id='r'`).Scan(&stamp); err != nil {
		t.Fatal(err)
	}
	result := run(`mutation {saveGlossaryTerm(input:{projectId:"p",name:"Генеральный подрядчик",definition:"Исполнитель",context:"",variants:[{text:"ГП",kind:"abbreviation"}]}){term{id}}}`, "saveGlossaryTerm").(map[string]any)
	termID := result["term"].(map[string]any)["id"].(string)
	restore := fmt.Sprintf(`mutation {restoreRequirement(projectId:"p",id:"r",version:1,archivedAt:%q)}`, stamp)
	if run(restore, "restoreRequirement") != true {
		t.Fatal("restore failed")
	}
	got := waitAPISummary(t, s, "token-a", "p", "r", "ready")
	if got["text"] != "Генеральный подрядчик Исполнитель" || !strings.Contains(got["provenance"].(string), termID) {
		t.Fatalf("restored summary still uses the previous glossary: %v", got)
	}
}
