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
		INSERT INTO requirements VALUES ('r','a','p','synthetic original',1,'now','now'),('q1','a','q','other project',1,'now','now'),('r2','b','foreign','other workspace',1,'now','now');`); err != nil {
		t.Fatal(err)
	}
	fields := `id status analysis limitations client model sharing createdAt sources { requirementId version status }`
	list := func(projectID, after string) string {
		var cursor string
		if after != "" {
			b, _ := json.Marshal(after)
			cursor = ",after:" + string(b)
		}
		return `{externalProposals(projectId:"` + projectID + `",first:1` + cursor + `){items{` + fields + `} pageInfo{hasNextPage endCursor}}}`
	}
	save := func(token, requestID, sources string) map[string]any {
		return summaryRequest(t, s, token, `mutation { saveExternalProposal(input:{
			projectId:"p",requestId:"`+requestID+`",analysis:"Check the policy",limitations:"Only a draft",
			client:"Example",model:"Example Model",sharing:"local",sources:`+sources+`
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
	initial := page(summaryRequest(t, s, "token-a", list("p", "")))
	if len(initial["items"].([]any)) != 0 {
		t.Fatal("rejected saves changed the list", initial)
	}
	var before int
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE 'project:p:external-proposals'=ANY(keys)`).Scan(&before); err != nil || before != 0 {
		t.Fatalf("invalid write published an event: %d %v", before, err)
	}
	saved := payload(save("token-a", "first", refs))["proposal"].(map[string]any)
	if saved["status"] != "pending_review" || saved["client"] != "Example" || saved["model"] != "Example Model" ||
		saved["sharing"] != "local" || saved["limitations"] != "Only a draft" ||
		saved["sources"].([]any)[0].(map[string]any)["status"] != "current" {
		t.Fatal(saved)
	}
	if got := payload(save("token-a", "first", refs))["proposal"].(map[string]any); got["id"] != saved["id"] {
		t.Fatal("retry created another proposal", got)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND 'project:p:external-proposals'=ANY(keys)`).Scan(&before); err != nil || before != 1 {
		t.Fatalf("expected one committed invalidation: %d %v", before, err)
	}
	if _, err = s.db.Exec(`UPDATE requirements SET text='synthetic edited',version=2 WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	if got := payload(save("token-a", "first", refs))["proposal"].(map[string]any); got["id"] != saved["id"] || got["sources"].([]any)[0].(map[string]any)["status"] != "stale" {
		t.Fatal("retry lost original after a source edit", got)
	}
	secondRefs := `[{requirementId:"r",version:2}]`
	if got := payload(save("token-a", "first", secondRefs)); got["proposal"] != nil || len(got["fieldErrors"].([]any)) == 0 {
		t.Fatal("request ID reuse accepted different content", got)
	}
	second := payload(save("token-a", "second", secondRefs))["proposal"].(map[string]any)
	s.Close()
	s, err = New(dsn, tokens)
	if err != nil {
		t.Fatal(err)
	}
	response := summaryRequest(t, s, "token-a", list("p", ""))
	firstPage := page(response)
	tags := response["extensions"].(map[string]any)["cacheTags"].([]any)
	if !slices.Contains(tags, "project:p:external-proposals") || !slices.Contains(tags, "project:p:requirements") {
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
	if _, err = s.db.Exec(`UPDATE requirements SET archived_at='now' WHERE id='r'; DELETE FROM requirements WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	deleted := page(summaryRequest(t, s, "token-a", list("p", after)))["items"].([]any)[0].(map[string]any)["sources"].([]any)[0].(map[string]any)
	if deleted["status"] != "unavailable" || deleted["requirementId"] != "r" || deleted["version"] != float64(1) {
		t.Fatal(deleted)
	}
}
