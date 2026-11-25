package api

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestRequirementConnectionLiteralMatch(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`
INSERT INTO projects VALUES
 ('p','a','P','G','','a','a'),
 ('other-a','a','Other A','G','','a','a'),
 ('other-b','b','Other B','G','','a','a');
INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at,archived_at)
VALUES
 ('r1','a','p','The deploy window starts at 20:00',1,'a','a',NULL),
 ('r2','a','p','Old DEPLOY window',1,'a','a','c'),
 ('r3','a','p','Verify schedule',1,'b','b',NULL),
 ('r4','a','p','Хранить 100% в dir_1/sub\file',1,'c','c',NULL),
 ('r5','a','p','Хранить 1000 в dirA1/subfile',1,'d','d',NULL),
 ('r6','a','p','Store a value in dirA1 subfile',1,'e','e',NULL),
 ('deploy-42','a','p','Schedule backups',1,'f','f',NULL),
 ('other-r1','a','other-a','Deploy window',1,'a','a',NULL),
 ('other-r2','b','other-b','Deploy window',1,'a','a',NULL);`); err != nil {
		t.Fatal(err)
	}

	query := func(t *testing.T, token, args string) map[string]any {
		t.Helper()
		return summaryRequest(t, s, token, `{ requirementsConnection(`+args+`){
			totalCount pageInfo{hasNextPage endCursor} edges{node{id}} } }`)
	}
	page := func(t *testing.T, token, args string, total int, hasNext bool, wantIDs ...string) string {
		t.Helper()
		result := query(t, token, args)
		if result["errors"] != nil {
			t.Fatalf("connection query rejected: %v", result["errors"])
		}
		connection := result["data"].(map[string]any)["requirementsConnection"].(map[string]any)
		pageInfo := connection["pageInfo"].(map[string]any)
		var ids []string
		for _, edge := range connection["edges"].([]any) {
			ids = append(ids, edge.(map[string]any)["node"].(map[string]any)["id"].(string))
		}
		if connection["totalCount"] != float64(total) || pageInfo["hasNextPage"] != hasNext || !slices.Equal(ids, wantIDs) {
			t.Fatalf("unexpected page for %s: %+v; IDs %v", args, connection, ids)
		}
		if len(wantIDs) == 0 {
			if pageInfo["endCursor"] != nil {
				t.Fatalf("unexpected cursor for an empty page: %v", pageInfo)
			}
			return ""
		}
		cursor, ok := pageInfo["endCursor"].(string)
		if !ok || cursor == "" {
			t.Fatalf("missing end cursor: %v", pageInfo)
		}
		return cursor
	}

	t.Run("literal text and IDs", func(t *testing.T) {
		for _, tc := range []struct {
			match string
			ids   []string
		}{
			{match: "   dEpLoY wInDoW  ", ids: []string{"r1", "r2"}},
			{match: "ХРАНИТЬ", ids: []string{"r4", "r5"}},
			{match: "DEPLOY-42", ids: []string{"deploy-42"}},
			{match: "DEPLOY-4", ids: []string{"deploy-42"}},
			{match: "%", ids: []string{"r4"}},
			{match: "_", ids: []string{"r4"}},
			{match: "/", ids: []string{"r4", "r5"}},
			{match: "\\", ids: []string{"r4"}},
			{match: "100% в dir_1/sub\\file", ids: []string{"r4"}},
			{match: "unmatched"},
		} {
			t.Run(tc.match, func(t *testing.T) {
				page(t, "token-a", fmt.Sprintf("projectId:\"p\",match:%q", tc.match), len(tc.ids), false, tc.ids...)
			})
		}
	})

	t.Run("archive and workspace filters", func(t *testing.T) {
		page(t, "token-a", `projectId:"p",match:"deploy window",archived:false`, 1, false, "r1")
		page(t, "token-a", `projectId:"p",match:"deploy window",archived:true`, 1, false, "r2")
		page(t, "token-a", `projectId:"p",match:"  "`, 6, false, "r1", "r3", "r4", "r5", "r6", "deploy-42")
		page(t, "token-a", `projectId:"p",match:"  ",archived:true`, 1, false, "r2")
		page(t, "token-b", `projectId:"p",match:"deploy window"`, 0, false)
		page(t, "token-a", `projectId:"other-b",match:"deploy window"`, 0, false)
	})

	t.Run("keyset pages and cursor scope", func(t *testing.T) {
		cursor := page(t, "token-a", `projectId:"p",match:" deploy window ",first:1`, 2, true, "r1")
		page(t, "token-a", fmt.Sprintf(`projectId:"p",match:"deploy window",first:1,after:%q`, cursor), 2, false, "r2")
		for _, scope := range []struct{ token, args string }{
			{token: "token-a", args: `projectId:"p",match:"deploy"`},
			{token: "token-a", args: `projectId:"p",match:"deploy window",archived:false`},
			{token: "token-a", args: `projectId:"p",match:"deploy window",archived:true`},
			{token: "token-a", args: `projectId:"p"`},
			{token: "token-a", args: `projectId:"other-a",match:"deploy window"`},
			{token: "token-b", args: `projectId:"p",match:"deploy window"`},
		} {
			result := query(t, scope.token, fmt.Sprintf(`%s,after:%q`, scope.args, cursor))
			if !strings.Contains(fmt.Sprint(result["errors"]), "invalid pagination cursor") {
				t.Errorf("cursor accepted across scope %s, %s: %v", scope.token, scope.args, result)
			}
		}
	})
}
