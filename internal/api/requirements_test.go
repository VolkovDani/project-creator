package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestRequirementLifecycleAndWorkspaceIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
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

func TestMigrationFromVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(1); CREATE TABLE projects(id TEXT PRIMARY KEY, workspace TEXT NOT NULL,name TEXT NOT NULL,goal TEXT NOT NULL,description TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL); INSERT INTO projects VALUES('old','a','P','G','','now','now');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := New(path, map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var name string
	if err = s.db.QueryRow(`SELECT name FROM projects WHERE id='old'`).Scan(&name); err != nil || name != "P" {
		t.Fatalf("data lost: %q %v", name, err)
	}
	var version int
	if err = s.db.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("migration: %d %v", version, err)
	}
}
