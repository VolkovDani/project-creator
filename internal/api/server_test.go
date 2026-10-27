package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"strings"
	"testing"
)

func TestProjectPersistenceAndIsolation(t *testing.T) {
	path := testDatabase(t)
	s, err := New(path, map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(token, query string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"query": query})
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	result := run("token-a", `mutation { createProject(input:{name:"A",goal:"B",description:""}) { project { id name } fieldErrors { code } } }`)
	if result["errors"] != nil {
		t.Fatal(result)
	}
	data := result["data"].(map[string]any)
	id := data["createProject"].(map[string]any)["project"].(map[string]any)["id"].(string)
	s.Close()
	s, err = New(path, map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	result = run("token-a", `{ project(id:"`+id+`") { name } }`)
	if result["errors"] != nil || result["data"].(map[string]any)["project"] == nil {
		t.Fatal(result)
	}
	result = run("token-b", `{ project(id:"`+id+`") { name } }`)
	if result["errors"] != nil || result["data"].(map[string]any)["project"] != nil {
		t.Fatal(result)
	}
	result = run("token-a", `mutation { createProject(input:{name:"",goal:"",description:""}) { project { id } fieldErrors { code path messageKey } } }`)
	if result["errors"] != nil || len(result["data"].(map[string]any)["createProject"].(map[string]any)["fieldErrors"].([]any)) != 2 {
		t.Fatal(result)
	}
}

func TestUnauthorized(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(`{"query":"{ projects { id } }"}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", w.Code)
	}
}

func TestProjectListPagination(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, q := range []string{`mutation { createProject(input:{name:"A",goal:"B",description:""}) { project { id } } }`, `mutation { createProject(input:{name:"C",goal:"D",description:""}) { project { id } } }`} {
		body, _ := json.Marshal(map[string]any{"query": q})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token-a")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
	}
	body, _ := json.Marshal(map[string]any{"query": `{ projects(limit:1, offset:1) { name } }`})
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-a")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["errors"] != nil || len(out["data"].(map[string]any)["projects"].([]any)) != 1 {
		t.Fatal(out)
	}
}

func TestRejectExcessiveAliases(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var q strings.Builder
	q.WriteString("{")
	for i := 0; i < 110; i++ {
		fmt.Fprintf(&q, " a%d:projects(limit:100,offset:0){id}", i)
	}
	q.WriteString("}")
	body, _ := json.Marshal(map[string]any{"query": q.String()})
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-a")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var result map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["errors"]) == 0 {
		t.Fatalf("excessive aliases accepted: %s", w.Body.String())
	}
}

func TestRejectFutureDatabaseVersion(t *testing.T) {
	path := testDatabase(t)
	db, err := sql.Open("pgx", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(2)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := New(path, map[string]string{"token-a": "a"})
	if err == nil {
		s.Close()
		t.Fatal("accepted newer database version")
	}
}

func TestExcessiveMutationIsRejectedBeforeAnyWrite(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var q strings.Builder
	q.WriteString("mutation {")
	for i := 0; i < 101; i++ {
		fmt.Fprintf(&q, ` a%d:createProject(input:{name:"A",goal:"B",description:""}){project{id}} `, i)
	}
	q.WriteString("}")
	body, _ := json.Marshal(map[string]any{"query": q.String()})
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-a")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM projects`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("wrote %d projects for oversized mutation", count)
	}
}
func TestExcessiveNestedAliasesRejected(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var q strings.Builder
	q.WriteString(`{ projects(limit:100,offset:0){`)
	for i := 0; i < 110; i++ {
		fmt.Fprintf(&q, " a%d:name", i)
	}
	q.WriteString("}}")
	body, _ := json.Marshal(map[string]any{"query": q.String()})
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-a")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var result map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result["errors"]) == 0 {
		t.Fatalf("accepted: %s", w.Body.String())
	}
}
