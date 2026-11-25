package api

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestExternalProposalsPreserveScopeAndSourceFreshness(t *testing.T) {
	dsn := testDatabase(t)
	tokens := map[string]string{"token-a": "a", "token-b": "b"}
	s, err := New(dsn, tokens)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if _, err := s.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'),('q','a','Q','G','','now','now'),('foreign','b','F','G','','now','now');
		INSERT INTO requirements VALUES ('r','a','p','synthetic original',1,'now','now'),('q1','a','q','other project',1,'now','now'),('r2','b','foreign','other workspace',1,'now','now');
		INSERT INTO glossary_terms(id,workspace,project_id,name,definition,meaning_context,variants,version,source_id,source_version) VALUES
		 ('term','a','p','Synthetic Original','A confirmed fixture','','[]',1,'',0),
		 ('term-q','a','q','Sibling Term','A sibling project fixture','','[]',1,'',0),
		 ('term-b','b','foreign','Foreign Term','A foreign workspace fixture','','[]',1,'',0);`); err != nil {
		t.Fatal(err)
	}
	fields := `id status analysis limitations client model sharing createdAt sources { requirementId version status } glossarySources { termId version status }`
	list := func(projectID, after string) string {
		var cursor string
		if after != "" {
			b, _ := json.Marshal(after)
			cursor = ",after:" + string(b)
		}
		return `{externalProposals(projectId:"` + projectID + `",first:1` + cursor + `){items{` + fields + `} pageInfo{hasNextPage endCursor}}}`
	}
	save := func(token, requestID, sources string, glossary ...string) map[string]any {
		var citations string
		if len(glossary) > 0 {
			citations = ",glossary:{sources:" + glossary[0] + "}"
		}
		return summaryRequest(t, s, token, `mutation { saveExternalProposal(input:{
			projectId:"p",requestId:"`+requestID+`",analysis:"Check the policy",limitations:"Only a draft",
			client:"Example",model:"Example Model",sharing:"local",sources:`+sources+citations+`
		}) { proposal {`+fields+`} fieldErrors { code path messageKey params { key value } } } }`)
	}
	payload := func(out map[string]any) map[string]any {
		t.Helper()
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)["saveExternalProposal"].(map[string]any)
	}
	page := func(out map[string]any) map[string]any {
		t.Helper()
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)["externalProposals"].(map[string]any)
	}
	if out := summaryRequest(t, s, "token-b", list("p", "")); out["errors"] == nil || strings.Contains(strings.TrimSpace(out["errors"].([]any)[0].(map[string]any)["message"].(string)), "synthetic") {
		t.Fatal("cross-workspace list accepted or disclosed data", out)
	}
	refs := `[{requirementId:"r",version:1}]`
	if out := save("token-b", "denied", refs); out["errors"] == nil {
		t.Fatal("cross-workspace save accepted", out)
	}
	for _, invalid := range []string{
		`[]`, `[{requirementId:"r",version:2}]`, `[{requirementId:"r2",version:1}]`, `[{requirementId:"q1",version:1}]`,
		`[{requirementId:"r",version:1},{requirementId:"r",version:1}]`,
	} {
		got := payload(save("token-a", "invalid", invalid))
		if got["proposal"] != nil || len(got["fieldErrors"].([]any)) == 0 {
			t.Fatal("invalid source accepted", invalid, got)
		}
	}
	for _, invalid := range []string{
		`[{termId:"term",version:2}]`, `[{termId:"term-q",version:1}]`,
		`[{termId:"term-b",version:1}]`, `[{termId:"term",version:1},{termId:"term",version:2}]`,
		`[{termId:"term",version:0}]`,
	} {
		got := payload(save("token-a", "invalid-glossary", refs, invalid))
		fields := got["fieldErrors"].([]any)
		if got["proposal"] != nil || len(fields) == 0 ||
			!slices.Contains(fields[0].(map[string]any)["path"].([]any), "glossary") {
			t.Fatal("invalid glossary source accepted", invalid, got)
		}
	}
	initial := page(summaryRequest(t, s, "token-a", list("p", "")))
	if len(initial["items"].([]any)) != 0 {
		t.Fatal("rejected saves changed the list", initial)
	}
	var before int
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE 'project:p:external-proposals'=ANY(keys)`).Scan(&before); err != nil || before != 0 {
		t.Fatalf("invalid write published an event: %d %v", before, err)
	}
	citation := `[{termId:"term",version:1}]`
	duplicate := `[{termId:"term",version:1},{termId:"term",version:1}]`
	saved := payload(save("token-a", "first", refs, duplicate))["proposal"].(map[string]any)
	if saved["status"] != "pending_review" || saved["client"] != "Example" || saved["model"] != "Example Model" ||
		saved["sharing"] != "local" || saved["limitations"] != "Only a draft" ||
		saved["sources"].([]any)[0].(map[string]any)["status"] != "current" {
		t.Fatal(saved)
	}
	checkCitation := func(item map[string]any, status string) {
		t.Helper()
		sources := item["glossarySources"].([]any)
		if len(sources) != 1 {
			t.Fatal("expected one cited term version", sources)
		}
		source := sources[0].(map[string]any)
		if source["termId"] != "term" || source["version"] != float64(1) || source["status"] != status {
			t.Fatal(source)
		}
	}
	checkCitation(saved, "current")
	if got := payload(save("token-a", "first", refs, citation))["proposal"].(map[string]any); got["id"] != saved["id"] {
		t.Fatal("retry created another proposal", got)
	}
	if got := payload(save("token-a", "first", refs)); got["proposal"] != nil || len(got["fieldErrors"].([]any)) == 0 {
		t.Fatal("request ID reuse accepted dropped glossary citation", got)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND 'project:p:external-proposals'=ANY(keys)`).Scan(&before); err != nil || before != 1 {
		t.Fatalf("expected one committed invalidation: %d %v", before, err)
	}
	if _, err = s.db.Exec(`UPDATE requirements SET text='synthetic edited',version=2 WHERE id='r'; UPDATE glossary_terms SET version=2 WHERE id='term'`); err != nil {
		t.Fatal(err)
	}
	retried := payload(save("token-a", "first", refs, citation))["proposal"].(map[string]any)
	if retried["id"] != saved["id"] || retried["sources"].([]any)[0].(map[string]any)["status"] != "stale" {
		t.Fatal("retry lost original after a source edit", retried)
	}
	checkCitation(retried, "stale")
	secondRefs := `[{requirementId:"r",version:2}]`
	if got := payload(save("token-a", "first", secondRefs)); got["proposal"] != nil || len(got["fieldErrors"].([]any)) == 0 {
		t.Fatal("request ID reuse accepted different content", got)
	}
	second := payload(save("token-a", "second", secondRefs))["proposal"].(map[string]any)
	if second["glossarySources"] == nil || len(second["glossarySources"].([]any)) != 0 {
		t.Fatal(second)
	}
	s.Close()
	s, err = New(dsn, tokens)
	if err != nil {
		t.Fatal(err)
	}
	response := summaryRequest(t, s, "token-a", list("p", ""))
	firstPage := page(response)
	tags := response["extensions"].(map[string]any)["cacheTags"].([]any)
	if !slices.Contains(tags, "project:p:external-proposals") || !slices.Contains(tags, "project:p:requirements") || !slices.Contains(tags, "project:p:glossary") {
		t.Fatal(tags)
	}
	info := firstPage["pageInfo"].(map[string]any)
	if info["hasNextPage"] != true || firstPage["items"].([]any)[0].(map[string]any)["id"] != second["id"] {
		t.Fatal(firstPage)
	}
	after := info["endCursor"].(string)
	if out := summaryRequest(t, s, "token-a", list("q", after)); out["errors"] == nil {
		t.Fatal("cross-project cursor accepted", out)
	}
	pageTwo := page(summaryRequest(t, s, "token-a", list("p", after)))
	original := pageTwo["items"].([]any)[0].(map[string]any)
	if original["analysis"] != "Check the policy" || original["sources"].([]any)[0].(map[string]any)["status"] != "stale" {
		t.Fatal(original)
	}
	checkCitation(original, "stale")
	if _, err = s.db.Exec(`UPDATE requirements SET archived_at='now' WHERE id='r'; DELETE FROM requirements WHERE id='r'; DELETE FROM glossary_terms WHERE id='term'`); err != nil {
		t.Fatal(err)
	}
	remaining := page(summaryRequest(t, s, "token-a", list("p", after)))["items"].([]any)[0].(map[string]any)
	checkCitation(remaining, "unavailable")
	deleted := remaining["sources"].([]any)[0].(map[string]any)
	if deleted["status"] != "unavailable" || deleted["requirementId"] != "r" || deleted["version"] != float64(1) {
		t.Fatal(deleted)
	}
}
