package api

import (
	"context"
	"fmt"
	"github.com/VolkovDani/project-creator/internal/explanation"
	"slices"
	"testing"
)

type explanationStub struct {
	collisionGenerator
	calls int
}

func (*explanationStub) ExplanationProfile() string { return "test-profile" }
func (g *explanationStub) ExplainClassification(_ context.Context, source explanation.Source, target explanation.Target) (explanation.Result, error) {
	g.calls++
	return explanation.Result{Meaning: explanation.Assessment{Verdict: "fits", Reason: target.Semantic}, Form: explanation.Assessment{Verdict: "partial", Reason: target.Representation}, Limitations: "One record."}, nil
}

func TestExplanationRequiresSavedModelEvidenceForCurrentSource(t *testing.T) {
	generator := &explanationStub{}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');
INSERT INTO requirements VALUES('r','a','p','The system prints receipts.',1,'now','now');
INSERT INTO requirement_classifications(workspace,project_id,requirement_id,revision,text_version,semantic_type,representation_type,status,source,rules_version) VALUES('a','p','r',1,1,'functional_requirement','statement','confirmed','user','2')`)
	if err != nil {
		t.Fatal(err)
	}
	call := func(token, query, field string) any {
		t.Helper()
		out := summaryRequest(t, s, token, query)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)[field]
	}
	const eligibility = `{classificationExplanationSuggestionRevision(projectId:"p",requirementId:"r")}`
	const field = "classificationExplanationSuggestionRevision"
	const mutation = `mutation {explainRequirementClassification(input:{projectId:"p",requirementId:"r",textVersion:1,revision:1,suggestionRevision:%d,semanticType:"functional_requirement",representationType:"user_story",locale:"en"}){status result{meaning{reason} form{reason}} fieldErrors{code}}}`
	for token, wantTag := range map[string]bool{"token-a": true, "token-b": false} {
		response := summaryRequest(t, s, token, eligibility)
		tags := response["extensions"].(map[string]any)["cacheTags"].([]any)
		if slices.Contains(tags, any("project:p:classification:r")) != wantTag {
			t.Fatalf("unexpected dependency for %s: %v", token, tags)
		}
	}
	if rev := call("token-a", eligibility, field); rev != float64(0) {
		t.Fatalf("manual-only record has proposal %v", rev)
	}
	if got := call("token-a", fmt.Sprintf(mutation, 0), "explainRequirementClassification").(map[string]any); got["status"] != "unclassified" || got["result"] != nil {
		t.Fatal(got)
	}
	_, err = s.db.Exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,revision,text_version,semantic_type,representation_type,status,source,rules_version,model,reanalysis) VALUES('a','p','r',2,1,'functional_requirement','statement','suggested','model','2','test-model',true)`)
	if err != nil {
		t.Fatal(err)
	}
	if rev := call("token-a", eligibility, field); rev != float64(2) {
		t.Fatalf("missing proposal %v", rev)
	}
	ready := call("token-a", fmt.Sprintf(mutation, 2), "explainRequirementClassification").(map[string]any)
	if ready["status"] != "ready" || generator.calls != 1 {
		t.Fatal(ready, generator.calls)
	}
	axes := ready["result"].(map[string]any)
	if axes["meaning"].(map[string]any)["reason"] != "functional_requirement" || axes["form"].(map[string]any)["reason"] != "user_story" {
		t.Fatal(axes)
	}
	if got := call("token-b", fmt.Sprintf(mutation, 2), "explainRequirementClassification").(map[string]any); got["result"] != nil || generator.calls != 1 {
		t.Fatal(got, generator.calls)
	}
	if rev := call("token-b", eligibility, field); rev != float64(0) {
		t.Fatalf("foreign proposal %v", rev)
	}
}
