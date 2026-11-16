package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestRequirementConnectionKeysetPages(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','2026-01-01','2026-01-01'); INSERT INTO requirements VALUES('r1','a','p','one',1,'a','a'),('r2','a','p','two',1,'b','b'),('r3','a','p','three',1,'c','c')`); err != nil {
		t.Fatal(err)
	}
	run := func(token, query string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": query})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["errors"] != nil {
			return map[string]any{"__errors": result["errors"]}
		}
		return result["data"].(map[string]any)
	}
	firstResult := run("token-a", `{requirementsConnection(projectId:"p",first:2){totalCount pageInfo{hasNextPage endCursor} edges{cursor node{id text}}}}`)
	if firstResult["__errors"] != nil {
		t.Fatalf("connection query rejected: %#v", firstResult["__errors"])
	}
	first := firstResult["requirementsConnection"].(map[string]any)
	if first["totalCount"] != float64(3) || first["pageInfo"].(map[string]any)["hasNextPage"] != true {
		t.Fatalf("unexpected first page: %#v", first)
	}
	cursor := first["pageInfo"].(map[string]any)["endCursor"].(string)
	second := run("token-a", fmt.Sprintf(`{requirementsConnection(projectId:"p",first:2,after:%q){totalCount pageInfo{hasNextPage} edges{cursor node{id}}}}`, cursor))["requirementsConnection"].(map[string]any)
	if second["totalCount"] != float64(3) || second["pageInfo"].(map[string]any)["hasNextPage"] != false || len(second["edges"].([]any)) != 1 {
		t.Fatalf("unexpected second page: %#v", second)
	}
	if got := run("token-b", `{requirementsConnection(projectId:"p"){totalCount edges{node{id}}}}`)["requirementsConnection"].(map[string]any); got["totalCount"] != float64(0) || len(got["edges"].([]any)) != 0 {
		t.Fatalf("workspace isolation failed: %#v", got)
	}
	if result := run("token-a", fmt.Sprintf(`{requirementsConnection(projectId:"other",after:%q){totalCount edges{node{id}}}}`, cursor)); result["__errors"] == nil {
		t.Fatalf("cursor accepted for another project: %#v", result)
	}
	projectCursor := run("token-a", `{projectsConnection(first:1){edges{cursor}}}`)["projectsConnection"].(map[string]any)["edges"].([]any)[0].(map[string]any)["cursor"].(string)
	if result := run("token-a", fmt.Sprintf(`{requirementsConnection(projectId:"p",after:%q){totalCount}}`, projectCursor)); result["__errors"] == nil {
		t.Fatalf("project cursor accepted for requirements connection: %#v", result)
	}
	if result := run("token-b", fmt.Sprintf(`{requirementsConnection(projectId:"p",after:%q){totalCount}}`, cursor)); result["__errors"] == nil {
		t.Fatalf("cursor accepted in another workspace: %#v", result)
	}
	for _, query := range []string{
		`{requirementsConnection(projectId:"p",first:0){totalCount}}`,
		`{requirementsConnection(projectId:"p",first:101){totalCount}}`,
		`{requirementsConnection(projectId:"p",after:"not-a-cursor"){totalCount}}`,
	} {
		if result := run("token-a", query); result["__errors"] == nil {
			t.Fatalf("invalid pagination accepted: %s -> %#v", query, result)
		}
	}
	empty := run("token-a", fmt.Sprintf(`{requirementsConnection(projectId:"p",after:%q){totalCount pageInfo{endCursor hasNextPage} edges{node{id}}}}`, second["edges"].([]any)[0].(map[string]any)["cursor"]))["requirementsConnection"].(map[string]any)
	if empty["totalCount"] != float64(3) || len(empty["edges"].([]any)) != 0 || empty["pageInfo"].(map[string]any)["hasNextPage"] != false || empty["pageInfo"].(map[string]any)["endCursor"] != nil {
		t.Fatalf("unexpected empty terminal page: %#v", empty)
	}
	if _, err = s.db.Exec(`DELETE FROM requirements WHERE id='r1'; INSERT INTO requirements VALUES('r0','a','p','earlier',1,'0','0'),('r4','a','p','later',1,'d','d')`); err != nil {
		t.Fatal(err)
	}
	changed := run("token-a", fmt.Sprintf(`{requirementsConnection(projectId:"p",first:2,after:%q){totalCount edges{node{id}}}}`, cursor))["requirementsConnection"].(map[string]any)
	changedIDs := changed["edges"].([]any)
	if changed["totalCount"] != float64(4) || len(changedIDs) != 2 || changedIDs[0].(map[string]any)["node"].(map[string]any)["id"] != "r3" || changedIDs[1].(map[string]any)["node"].(map[string]any)["id"] != "r4" {
		t.Fatalf("keyset did not remain stable after concurrent additions/deletions: %#v", changed)
	}
}

func TestProjectConnectionKeysetAndAnalysisPairFilters(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','a','a'),('q','a','Q','G','','a','a');
INSERT INTO requirements VALUES('r1','a','p','one',1,'a','a'),('r2','a','p','two',1,'b','b'),('r3','a','p','three',1,'c','c');
INSERT INTO analysis_runs(id,workspace,project_id,status,outcome,generator_profile,embedding_profile,source_signature,sources,lease_until) VALUES('run','a','p','completed','no_collisions_found','g','e','sig','[]',clock_timestamp());
INSERT INTO analysis_pairs(id,run_id,workspace,project_id,position,left_source,right_source,generator_profile,cache_key,status,verdict) VALUES
('pair1','run','a','p',1,'{"id":"r1","version":1,"text":"one"}','{"id":"r2","version":1,"text":"two"}','g','k1','completed','duplicate'),
('pair2','run','a','p',2,'{"id":"r1","version":1,"text":"one"}','{"id":"r3","version":1,"text":"three"}','g','k2','completed','collision'),
('pair3','run','a','p',3,'{"id":"r2","version":1,"text":"two"}','{"id":"r3","version":1,"text":"three"}','g','k3','completed','duplicate')`); err != nil {
		t.Fatal(err)
	}
	query := func(q string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": q})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token-a")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["errors"] != nil {
			return map[string]any{"__errors": result["errors"]}
		}
		return result["data"].(map[string]any)
	}
	projectPage := query(`{projectsConnection(first:1){totalCount pageInfo{hasNextPage endCursor} edges{node{id}}}}`)["projectsConnection"].(map[string]any)
	if projectPage["totalCount"] != float64(2) || projectPage["pageInfo"].(map[string]any)["hasNextPage"] != true {
		t.Fatalf("unexpected project page: %#v", projectPage)
	}
	cursor := projectPage["pageInfo"].(map[string]any)["endCursor"].(string)
	projectNext := query(fmt.Sprintf(`{projectsConnection(first:1,after:%q){edges{node{id}}}}`, cursor))["projectsConnection"].(map[string]any)
	if projectNext["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)["id"] != "q" {
		t.Fatalf("unexpected project continuation: %#v", projectNext)
	}
	pairPage := query(`{analysisPairsConnection(projectId:"p",analysisId:"run",first:1,status:"completed",verdict:"duplicate"){totalCount pageInfo{hasNextPage endCursor} edges{cursor node{id verdict}}}}`)["analysisPairsConnection"].(map[string]any)
	if pairPage["totalCount"] != float64(2) || pairPage["pageInfo"].(map[string]any)["hasNextPage"] != true || pairPage["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)["verdict"] != "duplicate" {
		t.Fatalf("analysis filters were not applied to the authorized count/page: %#v", pairPage)
	}
	pairCursor := pairPage["pageInfo"].(map[string]any)["endCursor"].(string)
	pairNext := query(fmt.Sprintf(`{analysisPairsConnection(projectId:"p",analysisId:"run",first:1,status:"completed",verdict:"duplicate",after:%q){edges{node{id verdict}} pageInfo{hasNextPage}}}`, pairCursor))["analysisPairsConnection"].(map[string]any)
	if pairNext["pageInfo"].(map[string]any)["hasNextPage"] != false || pairNext["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)["id"] != "pair3" {
		t.Fatalf("analysis pair keyset continuation failed: %#v", pairNext)
	}
	changedFilter := query(fmt.Sprintf(`{analysisPairsConnection(projectId:"p",analysisId:"run",status:"completed",verdict:"collision",after:%q){totalCount}}`, pairCursor))
	if changedFilter["__errors"] == nil {
		t.Fatalf("cursor was accepted after filter changed: %#v", changedFilter)
	}
}
