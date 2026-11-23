package api

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/classification"
	"strings"
	"testing"
)

func TestRequirementClassificationHistoryAndScope(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec("INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','source',1,'now','now')")
	if err != nil {
		t.Fatal(err)
	}
	read := `{requirementClassification(projectId:"p",requirementId:"r"){status semanticType representationType revision textVersion rulesVersion}}`
	call := func(token, query, field string) any {
		t.Helper()
		res := summaryRequest(t, s, token, query)
		if res["errors"] != nil {
			t.Fatal(res)
		}
		return res["data"].(map[string]any)[field]
	}
	get := func() map[string]any { return call("token-a", read, "requirementClassification").(map[string]any) }
	if get()["status"] != "unclassified" || call("token-b", read, "requirementClassification") != nil {
		t.Fatal("legacy default or scope")
	}
	_, err = s.db.Exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,revision,text_version,semantic_type,representation_type,status,source,rules_version)
		VALUES('a','p','r',1,1,'functional_requirement','scenario','confirmed','user','1')`)
	if err != nil {
		t.Fatal(err)
	}
	if legacy := get(); legacy["rulesVersion"] != "1" || legacy["representationType"] != "scenario" || legacy["status"] != "confirmed" {
		t.Fatal(legacy)
	}
	write := `mutation {confirmRequirementClassification(input:{projectId:"p",requirementId:"r",textVersion:1,revision:1,semanticType:"functional_requirement",representationType:"test_case"}){classification{status source rulesVersion representationType} fieldErrors{code}}}`
	bad := strings.Replace(write, "functional_requirement", "test_case", 1)
	if call("token-a", bad, "confirmRequirementClassification").(map[string]any)["classification"] != nil {
		t.Fatal("invalid axis")
	}
	if call("token-b", write, "confirmRequirementClassification").(map[string]any)["classification"] != nil {
		t.Fatal("foreign write")
	}
	payload := call("token-a", write, "confirmRequirementClassification").(map[string]any)
	result, ok := payload["classification"].(map[string]any)
	if !ok {
		t.Fatalf("test_case confirmation failed: %v", payload)
	}
	if result["status"] != "confirmed" || result["source"] != "user" || result["rulesVersion"] != "2" || result["representationType"] != "test_case" {
		t.Fatal(result)
	}
	if current := get(); current["revision"] != float64(2) || current["representationType"] != "test_case" || current["rulesVersion"] != "2" {
		t.Fatal(current)
	}
	filter := `{classifiedRequirements(projectId:"p",semanticType:"functional_requirement",representationType:"test_case",limit:10,offset:0){requirementId status}}`
	if len(call("token-a", filter, "classifiedRequirements").([]any)) != 1 || len(call("token-b", filter, "classifiedRequirements").([]any)) != 0 {
		t.Fatal("classification filter scope")
	}
	if len(call("token-a", strings.Replace(filter, "test_case", "scenario", 1), "classifiedRequirements").([]any)) != 0 {
		t.Fatal("representation filter")
	}
	if call("token-a", write, "confirmRequirementClassification").(map[string]any)["classification"] != nil {
		t.Fatal("lost update")
	}
	if _, err = s.db.Exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,revision,text_version,semantic_type,representation_type,status,source,rules_version,model)
		VALUES('a','p','r',3,1,'unknown','unknown','suggested','model','1','local')`); err == nil {
		t.Fatal("automatic suggestion replaced a confirmed decision")
	}
	if _, err = s.db.Exec("UPDATE requirements SET text='edited',version=2 WHERE id='r'"); err != nil {
		t.Fatal(err)
	}
	if get()["status"] != "stale" {
		t.Fatal(get())
	}
	history := call("token-a", `{requirementClassificationHistory(projectId:"p",requirementId:"r",limit:10,offset:0){status rulesVersion representationType}}`, "requirementClassificationHistory").([]any)
	if len(history) != 2 {
		t.Fatal(history)
	}
	current, legacy := history[0].(map[string]any), history[1].(map[string]any)
	if current["status"] != "confirmed" || current["rulesVersion"] != "2" || current["representationType"] != "test_case" ||
		legacy["status"] != "confirmed" || legacy["rulesVersion"] != "1" || legacy["representationType"] != "scenario" {
		t.Fatal(history)
	}
}

func TestClassificationDashboardWithoutProvider(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec("INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','source',1,'now','now'),('s','a','p','second',1,'now','now')")
	if err != nil {
		t.Fatal(err)
	}
	query := `{classificationProgress(projectId:"p"){total configured unclassified} classificationList(projectId:"p",status:"unclassified",limit:1,offset:0){requirementId text textVersion errorCode} classificationListByIds(projectId:"p",requirementIds:["s"]){requirementId text} classificationGroups(projectId:"p"){semanticType count}}`
	data := func(token, op string) map[string]any {
		t.Helper()
		result := summaryRequest(t, s, token, op)
		if result["errors"] != nil {
			t.Fatal(result["errors"])
		}
		return result["data"].(map[string]any)
	}
	d := data("token-a", query)
	progress := d["classificationProgress"].(map[string]any)
	if progress["total"] != float64(2) || progress["unclassified"] != float64(2) || progress["configured"] != false {
		t.Fatal(progress)
	}
	items := d["classificationList"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["errorCode"] != "NOT_CONFIGURED" {
		t.Fatal(items)
	}
	selected := d["classificationListByIds"].([]any)
	if len(selected) != 1 || selected[0].(map[string]any)["text"] != "second" {
		t.Fatal(selected)
	}
	if groups := d["classificationGroups"].([]any); len(groups) != 1 || groups[0].(map[string]any)["count"] != float64(2) {
		t.Fatal(groups)
	}
	foreign := data("token-b", query)
	if foreign["classificationProgress"] != nil || len(foreign["classificationListByIds"].([]any)) != 0 || len(foreign["classificationList"].([]any)) != 0 {
		t.Fatal(foreign)
	}
	mut := data("token-a", `mutation {backfillClassifications(projectId:"p"){queued fieldErrors{code messageKey}}}`)["backfillClassifications"].(map[string]any)
	if mut["queued"] != float64(0) || mut["fieldErrors"].([]any)[0].(map[string]any)["code"] != "NOT_CONFIGURED" {
		t.Fatal(mut)
	}
	if res := summaryRequest(t, s, "token-a", strings.Replace(query, `status:"unclassified"`, `status:"made_up"`, 1)); res["errors"] == nil {
		t.Fatal("invalid status accepted")
	}
}

func TestEmptyClassificationPageHasAuthorizedInvalidationTag(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec("INSERT INTO projects VALUES('p','a','P','G','','now','now')"); err != nil {
		t.Fatal(err)
	}
	query := `{filtered:classificationList(projectId:"p",status:"confirmed",limit:1,offset:0){requirementId}}`
	for _, tc := range []struct{ token, want string }{{"token-a", "project:p:classifications"}, {"token-b", "projects"}} {
		res := summaryRequest(t, s, tc.token, query)
		tags := res["extensions"].(map[string]any)["cacheTags"].([]any)
		if len(tags) != 1 || tags[0] != tc.want {
			t.Fatalf("%s: %v", tc.token, tags)
		}
	}
}

type dashboardClassifier struct{}

func (dashboardClassifier) Profile() string         { return "local-fixture-v2" }
func (dashboardClassifier) Model() string           { return "local-fixture" }
func (dashboardClassifier) Digest() string          { return "pinned-fixture" }
func (dashboardClassifier) QuestionVersion() string { return "questions-v2" }
func (dashboardClassifier) Classify(ctx context.Context, text string) (classification.Prediction, error) {
	if text != "Первая версия" {
		return classification.Prediction{}, fmt.Errorf("unexpected source")
	}
	score := 0.7
	return classification.Prediction{Status: "needs_review", Semantic: "unknown", Representation: "unknown", Reason: "UNCALIBRATED", Alternatives: []classification.Alternative{{RepresentationType: "statement", Score: &score}}}, nil
}

func TestRecordClassifierRunsAfterGraphQLCreate(t *testing.T) {
	s, err := NewConfigured(testDatabase(t), map[string]string{"token-a": "a"}, Config{Classifier: dashboardClassifier{}, DefaultAnalysisThreshold: contracts.AnalysisConstraints().DefaultThreshold})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(q, key string) map[string]any {
		t.Helper()
		res := summaryRequest(t, s, "token-a", q)
		if res["errors"] != nil {
			t.Fatal(res)
		}
		return res["data"].(map[string]any)[key].(map[string]any)
	}
	project := call(`mutation{createProject(input:{name:"P",goal:"G",description:""}){project{id}}}`, "createProject")["project"].(map[string]any)["id"].(string)
	item := call(fmt.Sprintf(`mutation{createRequirement(input:{projectId:%q,text:"Первая версия"}){requirement{id}}}`, project), "createRequirement")["requirement"].(map[string]any)["id"].(string)
	query := fmt.Sprintf(`{requirementClassification(projectId:%q,requirementId:%q){textVersion status digest profile questionVersion reason model source alternatives{representationType score}}}`, project, item)
	deadline := time.Now().Add(5 * time.Second)
	for {
		row := call(query, "requirementClassification")
		if row["status"] == "needs_review" {
			if row["source"] != "model" || row["digest"] != "pinned-fixture" || row["questionVersion"] != "questions-v2" || row["reason"] != "UNCALIBRATED" ||
				len(row["alternatives"].([]any)) != 1 || row["textVersion"] != float64(1) {
				t.Fatal(row)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("classification did not finish: %v", row)
		}
		time.Sleep(25 * time.Millisecond)
	}
	seen := call(fmt.Sprintf(`{classificationProgress(projectId:%q){configured needsReview queued classifying profile}}`, project), "classificationProgress")
	if seen["needsReview"] != float64(1) || seen["configured"] != true || seen["profile"] != "local-fixture-v2" {
		t.Fatal(seen)
	}
	bulk := call(fmt.Sprintf(`mutation {backfillClassifications(projectId:%q){queued fieldErrors{code}}}`, project), "backfillClassifications")
	if bulk["queued"] != float64(0) || len(bulk["fieldErrors"].([]any)) != 0 {
		t.Fatal(bulk)
	}
	wrong := call(fmt.Sprintf(`mutation {retryRequirementClassification(projectId:%q,requirementId:%q,textVersion:2,reanalyze:true){queued fieldErrors{path code}}}`, project, item), "retryRequirementClassification")
	if wrong["queued"] != float64(0) || wrong["fieldErrors"].([]any)[0].(map[string]any)["code"] != "CONFLICT" {
		t.Fatal(wrong)
	}
}

func TestManualClassificationScopeInvalidatedBySourceChanges(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec("INSERT INTO projects VALUES('p','a','P','G','','now','now')"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := readEventBatch(ctx, s.db, "a", eventCursor{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := readEventBatch(ctx, s.db, "b", eventCursor{})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"INSERT INTO requirements VALUES('r','a','p','source',1,'now','now')",
		"UPDATE requirements SET text='edited',version=2 WHERE id='r'",
		"DELETE FROM requirements WHERE id='r'",
	} {
		if _, err = s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
		a, err = readEventBatch(ctx, s.db, "a", a.cursor)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ev := range a.events {
			found = found || (slices.Contains(ev.keys, "project:p:classifications") && slices.Contains(ev.keys, "project:p:classification:r"))
		}
		if !found {
			t.Fatalf("source change failed to invalidate classification views: %s", stmt)
		}
	}
	b, err := readEventBatch(ctx, s.db, "b", other.cursor)
	if err != nil || len(b.events) != 0 {
		t.Fatalf("cross-workspace event: %+v %v", b, err)
	}
}

func TestFirstClassificationInvalidatesEmptyHistoryAndAggregate(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec("INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('r','a','p','source',1,'now','now')"); err != nil {
		t.Fatal(err)
	}
	history := `{history:requirementClassificationHistory(projectId:"p",requirementId:"r",limit:10,offset:0){status}}`
	for _, tc := range []struct {
		token, query string
		want         []string
	}{
		{"token-a", history, []string{"project:p:classification:r"}},
		{"token-b", history, []string{"projects"}},
		{"token-a", `{filtered:classifiedRequirements(projectId:"p",semanticType:"functional_requirement",limit:10,offset:0){status}}`, []string{"project:p:classifications"}},
		{"token-a", `mutation {saved:confirmRequirementClassification(input:{projectId:"p",requirementId:"r",textVersion:1,revision:0,semanticType:"functional_requirement",representationType:"statement"}){classification{status}}}`, []string{"project:p:classification:r", "project:p:classifications"}},
	} {
		res := summaryRequest(t, s, tc.token, tc.query)
		if res["errors"] != nil {
			t.Fatal(res)
		}
		tags := res["extensions"].(map[string]any)["cacheTags"].([]any)
		if len(tags) != len(tc.want) {
			t.Fatalf("%s: %v", tc.query, tags)
		}
		for i, key := range tc.want {
			if tags[i] != key {
				t.Fatalf("%s: %v", tc.query, tags)
			}
		}
	}
}
