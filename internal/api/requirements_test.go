package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"

	"testing"
)

func TestRequirementLifecycleAndWorkspaceIsolation(t *testing.T) {
	path := testDatabase(t)
	s, err := New(path, map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(token, q string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"query": q})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)
	}
	project := run("token-a", `mutation { createProject(input:{name:"P",goal:"G",description:""}){project{id}} }`)["createProject"].(map[string]any)["project"].(map[string]any)["id"].(string)
	query := fmt.Sprintf(`mutation { createRequirement(input:{projectId:"%s",text:"Первая версия"}) { requirement{id text version projectId} fieldErrors{code} } }`, project)
	created := run("token-a", query)["createRequirement"].(map[string]any)
	req := created["requirement"].(map[string]any)
	id := req["id"].(string)
	if req["version"] != float64(1) {
		t.Fatal(req)
	}
	denied := run("token-b", fmt.Sprintf(`{ requirements(projectId:"%s",limit:20,offset:0) { id } }`, project))
	if len(denied["requirements"].([]any)) != 0 {
		t.Fatal(denied)
	}
	cross := run("token-b", fmt.Sprintf(`mutation { updateRequirement(input:{id:"%s",projectId:"%s",version:1,text:"attacker"}) { requirement{id} } }`, id, project))
	if cross["updateRequirement"].(map[string]any)["requirement"] != nil {
		t.Fatal(cross)
	}
	deniedDelete := run("token-b", fmt.Sprintf(`mutation { deleteRequirement(id:"%s",projectId:"%s",version:2) { deleted } }`, id, project))
	if deniedDelete["deleteRequirement"].(map[string]any)["deleted"] != false {
		t.Fatal(deniedDelete)
	}
	invalid := run("token-a", fmt.Sprintf(`mutation { createRequirement(input:{projectId:"%s",text:" "}) { fieldErrors{code} } }`, project))
	if len(invalid["createRequirement"].(map[string]any)["fieldErrors"].([]any)) != 1 {
		t.Fatal(invalid)
	}
	result := run("token-a", fmt.Sprintf(`mutation { updateRequirement(input:{id:"%s",projectId:"%s",version:1,text:"Вторая версия"}) { requirement{text version} } }`, id, project))
	u := result["updateRequirement"].(map[string]any)["requirement"].(map[string]any)
	if u["text"] != "Вторая версия" || u["version"] != float64(2) {
		t.Fatal(result)
	}
	conflict := run("token-a", fmt.Sprintf(`mutation { updateRequirement(input:{id:"%s",projectId:"%s",version:1,text:"stale"}) { fieldErrors { code } } }`, id, project))
	fields := conflict["updateRequirement"].(map[string]any)["fieldErrors"].([]any)
	if len(fields) != 1 || fields[0].(map[string]any)["code"] != "CONFLICT" {
		t.Fatal(conflict)
	}
	staleDelete := run("token-a", fmt.Sprintf(`mutation { deleteRequirement(id:"%s",projectId:"%s",version:1) { deleted } }`, id, project))
	if staleDelete["deleteRequirement"].(map[string]any)["deleted"] != false {
		t.Fatal(staleDelete)
	}
	s.Close()
	s, err = New(path, map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	found := run("token-a", fmt.Sprintf(`{ requirements(projectId:"%s",limit:20,offset:0) { id text version } }`, project))
	if len(found["requirements"].([]any)) != 1 {
		t.Fatal(found)
	}
	deleted := run("token-a", fmt.Sprintf(`mutation { deleteRequirement(id:"%s",projectId:"%s",version:2) { deleted } }`, id, project))
	if deleted["deleteRequirement"].(map[string]any)["deleted"] != false {
		t.Fatal("active requirement was permanently deleted", deleted)
	}
	confirmClass := fmt.Sprintf(`mutation {confirmRequirementClassification(input:{projectId:%q,requirementId:%q,textVersion:2,revision:0,semanticType:"functional_requirement",representationType:"statement"}){classification{status}}}`, project, id)
	if run("token-a", confirmClass)["confirmRequirementClassification"].(map[string]any)["classification"].(map[string]any)["status"] != "confirmed" {
		t.Fatal("classification not saved")
	}
	classification := fmt.Sprintf(`{requirementClassification(projectId:%q,requirementId:%q){status}}`, project, id)
	move := fmt.Sprintf(`mutation { archiveRequirement(id:%q,projectId:%q,version:2) }`, id, project)
	if run("token-b", move)["archiveRequirement"] != false || run("token-a", move)["archiveRequirement"] != true {
		t.Fatal("archive transition or authorization failed")
	}
	list := fmt.Sprintf(`{ requirementsConnection(projectId:%q,archived:true){totalCount edges{node{id text version archivedAt}}} }`, project)
	stamp := func() string {
		archive := run("token-a", list)["requirementsConnection"].(map[string]any)
		items := archive["edges"].([]any)
		if len(items) != 1 {
			t.Fatal(archive)
		}
		node := items[0].(map[string]any)["node"].(map[string]any)
		if node["id"] != id || node["version"] != float64(2) || node["text"] != "Вторая версия" {
			t.Fatal(node)
		}
		return node["archivedAt"].(string)
	}
	if run("token-b", list)["requirementsConnection"].(map[string]any)["totalCount"] != float64(0) {
		t.Fatal("cross-workspace archive")
	}
	if run("token-a", classification)["requirementClassification"] != nil {
		t.Fatal("archived requirement in active classification")
	}
	firstStamp := stamp()
	unconfirmed := fmt.Sprintf(`mutation { deleteRequirement(id:%q,projectId:%q,version:2,archivedAt:%q){deleted} }`, id, project, firstStamp)
	if run("token-a", unconfirmed)["deleteRequirement"].(map[string]any)["deleted"] != false {
		t.Fatal("archive entry deleted without explicit confirmation")
	}
	if run("token-a", fmt.Sprintf(`{requirementsConnection(projectId:%q){totalCount}}`, project))["requirementsConnection"].(map[string]any)["totalCount"] != float64(0) {
		t.Fatal("archived requirement still active")
	}
	restore := fmt.Sprintf(`mutation { restoreRequirement(id:%q,projectId:%q,version:2,archivedAt:%q) }`, id, project, firstStamp)
	if run("token-a", restore)["restoreRequirement"] != true || run("token-a", classification)["requirementClassification"].(map[string]any)["status"] != "confirmed" || run("token-a", move)["archiveRequirement"] != true {
		t.Fatal("restore/rearchive failed")
	}
	purge := func(token, when string) bool {
		q := fmt.Sprintf(`mutation { deleteRequirement(id:%q,projectId:%q,version:2,archivedAt:%q,confirmed:true){deleted} }`, id, project, when)
		return run(token, q)["deleteRequirement"].(map[string]any)["deleted"].(bool)
	}
	if purge("token-a", firstStamp) || purge("token-b", stamp()) || !purge("token-a", stamp()) {
		t.Fatal("invalid purge guard")
	}
	if run("token-a", list)["requirementsConnection"].(map[string]any)["totalCount"] != float64(0) {
		t.Fatal("purge did not remove archive entry")
	}
	found = run("token-a", fmt.Sprintf(`{ requirements(projectId:"%s",limit:20,offset:0) { id } }`, project))
	if len(found["requirements"].([]any)) != 0 {
		t.Fatal(found)
	}
}

func TestRequirementPaginationAndProjectIsolation(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','a','Q','G','','now','now'); INSERT INTO requirements VALUES('r1','a','p','first',1,'a','a'),('r2','a','p','second',1,'b','b'),('r3','a','q','hidden',1,'a','a')`); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"query": `{requirements(projectId:"p",limit:1,offset:1){id text}}`})
	request := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer token-a")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	var result struct {
		Data struct {
			Requirements []struct {
				ID   string
				Text string
			}
		}
		Errors []any
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 || len(result.Data.Requirements) != 1 || result.Data.Requirements[0].ID != "r2" {
		t.Fatalf("unexpected page: %s", response.Body.String())
	}
}
