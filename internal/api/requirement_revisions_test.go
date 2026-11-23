package api

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func TestRequirementRevisionHistory(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec("INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','a','Q','G','','now','now')"); err != nil {
		t.Fatal(err)
	}
	field := func(t *testing.T, token, query, name string) any {
		t.Helper()
		response := summaryRequest(t, s, token, query)
		if response["errors"] != nil {
			t.Fatal(response)
		}
		return response["data"].(map[string]any)[name]
	}
	created := field(t, "token-a", `mutation{createRequirement(input:{projectId:"p",text:"One"}){requirement{id createdAt}}}`, "createRequirement").(map[string]any)["requirement"].(map[string]any)
	id := created["id"].(string)
	save := func(t *testing.T, token, project string, version int, text, reasonInput string) map[string]any {
		t.Helper()
		return field(t, token, fmt.Sprintf(`mutation{updateRequirement(input:{id:%q,projectId:%q,version:%d,text:%q %s}){requirement{text version updatedAt} fieldErrors{path code messageKey params{key value}}}}`, id, project, version, text, reasonInput), "updateRequirement").(map[string]any)
	}
	history := func(token, project string, first int, cursor string) []any {
		t.Helper()
		return field(t, token, fmt.Sprintf(`{requirementRevisionHistory(requirementId:%q,projectId:%q,first:%d %s){version previousText text changeReason author createdAt}}`, id, project, first, cursor), "requirementRevisionHistory").([]any)
	}
	for _, reason := range []string{"", "changeReason:null", `changeReason:" \t  "`} {
		result := save(t, "token-a", "p", 1, "Two", reason)
		want := []any{map[string]any{"path": []any{"input", "changeReason"}, "code": "REQUIRED", "messageKey": "validation.required", "params": []any{}}}
		if result["requirement"] != nil || !reflect.DeepEqual(result["fieldErrors"], want) {
			t.Fatalf("missing reason %q: %v", reason, result)
		}
	}
	for _, scope := range []struct{ token, project string }{{"token-b", "p"}, {"token-a", "q"}} {
		if save(t, scope.token, scope.project, 1, "Two", `changeReason:"test"`)["requirement"] != nil || len(history(scope.token, scope.project, 10, "")) != 0 {
			t.Fatal("scope leak", scope)
		}
	}
	type attempt struct {
		reason, text string
		result       map[string]any
	}
	attempts := []attempt{{text: "Two"}, {text: "Second"}}
	t.Run("competing updates", func(t *testing.T) {
		for i := range attempts {
			t.Run(attempts[i].text, func(t *testing.T) {
				t.Parallel()
				attempt := &attempts[i]
				attempt.reason = "  Исправление: " + attempt.text + "\n"
				attempt.result = save(t, "token-a", "p", 1, attempt.text, fmt.Sprintf("changeReason:%q", attempt.reason))
			})
		}
	})
	var winner attempt
	wins, conflicts := 0, 0
	for _, result := range attempts {
		if result.result == nil {
			t.Fatal("missing update response")
		}
		if result.result["requirement"] != nil {
			winner = result
			wins++
		} else if errors := result.result["fieldErrors"].([]any); len(errors) == 1 && errors[0].(map[string]any)["code"] == "CONFLICT" {
			conflicts++
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins %d conflicts %d", wins, conflicts)
	}
	if entry := save(t, "token-a", "p", 2, winner.text, `changeReason:"Reviewed again"`); entry["requirement"] == nil {
		t.Fatal(entry)
	}
	revisions := history("token-a", "p", 2, "")
	if len(revisions) != 2 {
		t.Fatal(revisions)
	}
	newest, edited := revisions[0].(map[string]any), revisions[1].(map[string]any)
	if newest["version"] != float64(3) || newest["previousText"] != winner.text || newest["text"] != winner.text || newest["changeReason"] != "Reviewed again" {
		t.Fatal(newest)
	}
	if edited["version"] != float64(2) || edited["previousText"] != "One" || edited["text"] != winner.text || edited["changeReason"] != winner.reason || edited["author"] != nil || edited["createdAt"] != winner.result["requirement"].(map[string]any)["updatedAt"] {
		t.Fatal(edited)
	}
	if _, err := time.Parse(time.RFC3339Nano, edited["createdAt"].(string)); err != nil {
		t.Fatal(err)
	}
	older := history("token-a", "p", 2, "beforeVersion:2")
	initial := map[string]any{"version": float64(1), "text": "One", "previousText": nil, "changeReason": nil, "author": nil, "createdAt": created["createdAt"]}
	if !reflect.DeepEqual(older, []any{initial}) || len(history("token-a", "p", 2, "beforeVersion:1")) != 0 {
		t.Fatal(older)
	}
	for _, page := range []string{"first:0", "first:101", "first:1 beforeVersion:0"} {
		response := summaryRequest(t, s, "token-a", fmt.Sprintf(`{requirementRevisionHistory(projectId:"p",requirementId:%q,%s){version}}`, id, page))
		if response["errors"] == nil {
			t.Errorf("invalid page %s: %v", page, response)
		}
	}
	// Failed revision inserts must roll back the source update and its notification.
	eventCount := func() int {
		t.Helper()
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM cache_events WHERE workspace='a' AND 'project:p:requirements'=ANY(keys)").Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	before := eventCount()
	if _, err = s.db.Exec("ALTER TABLE requirement_text_revisions ADD CHECK (version < 4)"); err != nil {
		t.Fatal(err)
	}
	response := summaryRequest(t, s, "token-a", fmt.Sprintf(`mutation{updateRequirement(input:{id:%q,projectId:"p",version:3,text:"Rolled back",changeReason:"test"}){requirement{version}}}`, id))
	if response["errors"] == nil {
		t.Fatal(response)
	}
	var current int
	if err = s.db.QueryRow("SELECT version FROM requirements WHERE id=$1", id).Scan(&current); err != nil || current != 3 {
		t.Fatalf("version %d error %v", current, err)
	}
	if len(history("token-a", "p", 10, "")) != 3 || eventCount() != before {
		t.Fatal("rollback changed history")
	}
	deleted := field(t, "token-a", fmt.Sprintf(`mutation{deleteRequirement(projectId:"p", id:%q, version:3){deleted}}`, id), "deleteRequirement").(map[string]any)
	if deleted["deleted"] != true || len(history("token-a", "p", 10, "")) != 0 {
		t.Fatal(deleted)
	}
}

func TestRequirementRevisionUpgrade(t *testing.T) {
	db := analysisDatabaseAtVersion(t, 1)
	if _, err := db.Exec("INSERT INTO projects VALUES('p','a','P','G','','then','then'); INSERT INTO requirements VALUES('r','a','p','known text',8,'earlier','updated')"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := storage.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	var total, matched int
	if err := db.QueryRow(`SELECT count(*),count(*) FILTER (WHERE version=8 AND text='known text' AND created_at='updated' AND change_reason IS NULL AND author IS NULL) FROM requirement_text_revisions WHERE requirement_id='r'`).Scan(&total, &matched); err != nil || total != 1 || matched != 1 {
		t.Fatalf("legacy snapshots %d/%d: %v", matched, total, err)
	}
}
