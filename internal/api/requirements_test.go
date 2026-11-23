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
	cross := run("token-b", fmt.Sprintf(`mutation { updateRequirement(input:{id:"%s",projectId:"%s",version:1,changeReason:"Clarify the requirement",text:"attacker"}) { requirement{id} } }`, id, project))
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
	result := run("token-a", fmt.Sprintf(`mutation { updateRequirement(input:{id:"%s",projectId:"%s",version:1,changeReason:"Clarify the requirement",text:"Вторая версия"}) { requirement{text version} } }`, id, project))
	u := result["updateRequirement"].(map[string]any)["requirement"].(map[string]any)
	if u["text"] != "Вторая версия" || u["version"] != float64(2) {
		t.Fatal(result)
	}
	conflict := run("token-a", fmt.Sprintf(`mutation { updateRequirement(input:{id:"%s",projectId:"%s",version:1,changeReason:"Clarify the requirement",text:"stale"}) { fieldErrors { code } } }`, id, project))
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
	restore := func(token, projectID string, version int, when string) bool {
		q := fmt.Sprintf(`mutation { restoreRequirement(id:%q,projectId:%q,version:%d,archivedAt:%q) }`, id, projectID, version, when)
		return run(token, q)["restoreRequirement"].(bool)
	}
	purge := func(token, projectID string, version int, when string) bool {
		q := fmt.Sprintf(`mutation { deleteRequirement(id:%q,projectId:%q,version:%d,archivedAt:%q,confirmed:true){deleted} }`, id, projectID, version, when)
		return run(token, q)["deleteRequirement"].(map[string]any)["deleted"].(bool)
	}
	otherProject := run("token-a", `mutation {createProject(input:{name:"Other",goal:"G",description:""}){project{id}}}`)["createProject"].(map[string]any)["project"].(map[string]any)["id"].(string)
	for _, guard := range []struct {
		token, projectID string
		version          int
	}{{"token-a", project, 1}, {"token-b", project, 2}, {"token-a", otherProject, 2}} {
		if restore(guard.token, guard.projectID, guard.version, firstStamp) || purge(guard.token, guard.projectID, guard.version, firstStamp) {
			t.Fatalf("unauthorized or stale transition: %+v", guard)
		}
	}
	if !restore("token-a", project, 2, firstStamp) || run("token-a", classification)["requirementClassification"].(map[string]any)["status"] != "confirmed" || run("token-a", move)["archiveRequirement"] != true {
		t.Fatal("restore/rearchive failed")
	}
	if restore("token-a", project, 2, firstStamp) || purge("token-a", project, 2, firstStamp) || !purge("token-a", project, 2, stamp()) {
		t.Fatal("invalid archive timestamp guard")
	}
	var revisions int
	if err := s.db.QueryRow("SELECT count(*) FROM requirement_text_revisions WHERE requirement_id=$1", id).Scan(&revisions); err != nil || revisions != 0 {
		t.Fatalf("purged revisions: %d, %v", revisions, err)
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

func TestArchiveRestoresEligibleDerivedWork(t *testing.T) {
	db := eventDB(t)
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`
		INSERT INTO projects VALUES('p','a','P','G','','now','now');
		INSERT INTO summary_profiles VALUES('a','A','provenance');
		INSERT INTO embedding_profiles(workspace,profile) VALUES('a','A');
		INSERT INTO classification_profiles VALUES('a','A','model','digest','v1','v1');
		INSERT INTO requirements VALUES
		 ('resume','a','p','one',1,'now','now'),('finished','a','p','two',2,'now','now'),('cancelled','a','p','three',3,'now','now');
		UPDATE requirement_summaries SET text='preview',status='ready' WHERE requirement_id='finished';
		UPDATE requirement_summaries SET status='cancelled' WHERE requirement_id='cancelled';
		UPDATE requirement_index SET status='ready' WHERE requirement_id='finished';
		INSERT INTO requirement_classifications(requirement_id,workspace,project_id,revision,text_version,semantic_type,representation_type,status,source,rules_version)
		 VALUES('finished','a','p',1,2,'functional_requirement','statement','confirmed','user','v1');
		UPDATE requirements SET archived_at='archived';`)
	// Profile activation can mark archived work stale. Its normal backfill only scans active sources.
	exec(`
		UPDATE requirement_summaries SET status='stale',provenance='old provenance' WHERE status='queued';
		UPDATE classification_jobs SET status='stale' WHERE status='queued';
		UPDATE requirement_index SET status='failed',error_code='PROFILE_CHANGED' WHERE status='queued';
		DELETE FROM classification_jobs WHERE requirement_id='finished';
		UPDATE requirements SET archived_at=NULL;`)
	for _, test := range []struct {
		name, query string
		want        int
	}{
		{"summary", `SELECT count(*) FROM requirement_summaries WHERE
		  (requirement_id='resume' AND status='queued' AND provenance='provenance') OR
		  (requirement_id='finished' AND status='ready' AND text='preview') OR
		  (requirement_id='cancelled' AND status='cancelled')`, 3},
		{"index", `SELECT count(*) FROM requirement_index WHERE (requirement_id='finished' AND status='ready')
		  OR (requirement_id IN ('resume','cancelled') AND status='queued' AND error_code='')`, 3},
		{"classification", `SELECT count(*) FROM classification_jobs WHERE requirement_id IN ('resume','cancelled') AND status='queued'`, 2},
		{"confirmed", `SELECT count(*) FROM classification_jobs WHERE requirement_id='finished'`, 0},
	} {
		var n int
		if err := db.QueryRow(test.query).Scan(&n); err != nil || n != test.want {
			t.Errorf("%s: rows %d, want %d, error %v", test.name, n, test.want, err)
		}
	}
}
