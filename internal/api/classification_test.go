package api

import (
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
