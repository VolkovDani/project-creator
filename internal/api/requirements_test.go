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
	if deleted["deleteRequirement"].(map[string]any)["deleted"] != true {
		t.Fatal(deleted)
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
