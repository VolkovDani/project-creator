package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/explanation"
)

type explanationStub struct {
	collisionGenerator
	calls int
}

func TestManagedNPUAssessmentThroughGraphQL(t *testing.T) {
	exerciseManagedNPUAssessment(t, "")
}

func TestModelManagerAssessmentIntegration(t *testing.T) {
	binary := os.Getenv("MODELS_CONFIGURATOR_TEST_BIN")
	if binary == "" {
		t.Skip("set MODELS_CONFIGURATOR_TEST_BIN to run the cross-repository test")
	}
	exerciseManagedNPUAssessment(t, binary)
}

func exerciseManagedNPUAssessment(t *testing.T, managerBinary string) {
	t.Helper()
	database := testDatabase(t)
	const firstPassage = "Система печатает квитанции по завершённым продажам."
	const secondPassage = "Кассир выбирает операцию за текущую смену."
	const text = firstPassage + " " + secondPassage + " " +
		"На бумаге указываются дата продажи, номер операции, итоговая сумма, валюта и способ оплаты. " +
		"Если принтер занят, система показывает состояние задания и держит его в очереди. " +
		"Кассир использует квитанцию для сверки операции с отчётом по смене."
	const meaningReason = "Печать квитанции описывает работу системы."
	const formReason = "Требование сформулировано как утверждение."
	const meaningCriterion = "system_behavior"
	const formCriterion = "declarative_assertion_or_directive"
	digest := strings.Repeat("a", 64)
	model := "openvino-npu.assessment." + digest
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managerBinary != "" && r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{"profile": digest, "device": "NPU"})
			return
		}
		call := calls.Add(1)
		if call > 2 {
			t.Errorf("unexpected extra assessment request %d", call)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var request struct {
			Model          string
			Profile        string
			Messages       []struct{ Role, Content string }
			Schema         json.RawMessage
			ResponseFormat struct {
				JSONSchema struct{ Schema json.RawMessage } `json:"json_schema"`
			} `json:"response_format"`
		}
		err := json.NewDecoder(r.Body).Decode(&request)
		schema := request.Schema
		var bound bool
		if managerBinary != "" {
			bound = r.URL.Path == "/v1/generate" && request.Profile == digest
		} else {
			bound = r.URL.Path == "/v1/chat/completions" && request.Model == model
			schema = request.ResponseFormat.JSONSchema.Schema
		}
		if err != nil || !bound || len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Error("unexpected model request", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		axis, ownCriterion, otherCriterion := "content", meaningCriterion, formCriterion
		reason, otherReason := meaningReason, formReason
		if call == 2 {
			axis, ownCriterion, otherCriterion = "form", formCriterion, meaningCriterion
			reason, otherReason = formReason, meaningReason
		}
		system, user := request.Messages[0].Content, request.Messages[1].Content
		var output struct{ Properties map[string]json.RawMessage }
		if !strings.Contains(system, `"axis":"`+axis+`"`) || !strings.Contains(system, ownCriterion) ||
			strings.Contains(system, otherCriterion) || strings.Contains(user, otherCriterion) ||
			strings.Contains(system, otherReason) || strings.Contains(user, otherReason) ||
			json.Unmarshal(schema, &output) != nil || len(output.Properties) != 4 || output.Properties["assessment"] == nil ||
			output.Properties["questions"] == nil || output.Properties["wording"] == nil || output.Properties["limitations"] == nil ||
			!strings.Contains(string(output.Properties["assessment"]), ownCriterion) || strings.Contains(string(schema), otherCriterion) {
			t.Errorf("model request %d did not isolate %s", call, axis)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct {
			SourcePassages []struct {
				ID   int
				Text string
			}
			SavedDecision json.RawMessage
			SavedProposal string
		}
		if err := json.Unmarshal([]byte(user), &input); err != nil || input.SavedProposal != "unknown" ||
			(len(input.SavedDecision) > 0 && string(input.SavedDecision) != "null") || strings.Contains(user, `"alternatives"`) {
			t.Errorf("model request %d included unexpected classification context: %v", call, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var source strings.Builder
		for i, passage := range input.SourcePassages {
			if passage.ID != i+1 {
				t.Errorf("model request %d renumbered its source passages", call)
			}
			source.WriteString(passage.Text)
		}
		if source.String() != text {
			t.Errorf("model request %d changed the source text", call)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		content := fmt.Sprintf(`{
			"assessment":{"verdict":"fits","reason":%q,"criteria":{%q:{"status":"present","evidence":%d,"note":"Признак явно указан."}}},
			"questions":[],"wording":"","limitations":"Оценено одно требование."
		}`, reason, ownCriterion, call)
		if managerBinary != "" {
			json.NewEncoder(w).Encode(map[string]string{"profile": digest, "device": "NPU", "content": content})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"model": model, "choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": content},
		}}})
	})
	var base string
	if managerBinary == "" {
		provider := httptest.NewServer(handler)
		defer provider.Close()
		base = provider.URL + "/v1"
	} else {
		socket := filepath.Join(t.TempDir(), "npu.sock")
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		runtime := &http.Server{Handler: handler}
		go runtime.Serve(listener)
		t.Cleanup(func() { runtime.Close() })
		var models []testManagerModel
		base, models = startTestManager(t, managerBinary, []map[string]string{
			{"name": "assessment", "provider": "openvino-npu", "base_url": "unix://" + socket, "role": "chat", "model": "stub", "digest": digest},
		}, nil)
		if len(models) != 1 || models[0].Role != "chat" {
			t.Fatalf("unexpected NPU inventory: %+v", models)
		}
		model = models[0].ID
	}
	g, err := analysis.NewOpenAICompatible(base, model, "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithProviders(database, map[string]string{"token-a": "a", "token-b": "b"}, nil, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`INSERT INTO requirements VALUES('r','a','p',$1,1,'now','now')`, text)
	if err != nil {
		t.Fatal(err)
	}
	rules := contracts.ClassificationRules()
	score := 0.0839123456721
	var ranked []classification.Alternative
	for _, meaning := range rules.Semantic {
		ranked = append(ranked, classification.Alternative{SemanticType: meaning, Score: &score})
	}
	for _, form := range rules.Representation {
		ranked = append(ranked, classification.Alternative{RepresentationType: form, Score: &score})
	}
	alternatives, err := json.Marshal(ranked)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`INSERT INTO requirement_classifications(workspace,project_id,requirement_id,revision,text_version,semantic_type,representation_type,status,source,rules_version,model,alternatives)
VALUES('a','p','r',1,1,'unknown','unknown','needs_review','model','2','test-model',$1)`, alternatives)
	if err != nil {
		t.Fatal(err)
	}
	const mutation = `mutation {
		explainRequirementClassification(input:{projectId:"p", requirementId:"r", textVersion:1, revision:1, suggestionRevision:1, semanticType:"functional_requirement", representationType:"statement", locale:"ru"}) {
			status result { meaning { verdict criteria { evidence } } form { verdict criteria { evidence } } }
		}
	}`
	for _, token := range []string{"token-b", "token-a", "token-a"} {
		res := summaryRequest(t, s, token, mutation)
		data, _ := res["data"].(map[string]any)
		got, _ := data["explainRequirementClassification"].(map[string]any)
		if token == "token-b" {
			if got["result"] != nil || calls.Load() != 0 {
				t.Fatal("foreign workspace reached the inference adapter", res)
			}
			continue
		}
		result, _ := got["result"].(map[string]any)
		meaning, _ := result["meaning"].(map[string]any)
		form, _ := result["form"].(map[string]any)
		meaningCriteria, _ := json.Marshal(meaning["criteria"])
		formCriteria, _ := json.Marshal(form["criteria"])
		if got["status"] != "ready" || meaning["verdict"] != "fits" || form["verdict"] != "fits" ||
			!strings.Contains(string(meaningCriteria), `"evidence":"`+firstPassage+`"`) ||
			!strings.Contains(string(formCriteria), `"evidence":"`+secondPassage+`"`) || calls.Load() != 2 {
			t.Fatal("expected a grounded and cached assessment", res, calls.Load())
		}
	}
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
