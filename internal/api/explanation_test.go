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
	const thirdPassage = "На бумаге указываются дата продажи, номер операции, итоговая сумма, валюта и способ оплаты."
	const fourthPassage = "Если принтер занят, система показывает состояние задания и держит его в очереди."
	const text = firstPassage + " " + secondPassage + " " +
		thirdPassage + " " + fourthPassage + " " +
		"Кассир использует квитанцию для сверки операции с отчётом по смене."
	expectedCalls := [...]struct {
		axis, category, reason string
		criteria               []string
	}{
		{"content", "functional_requirement", "Печать квитанции описывает работу системы.", []string{"system_behavior"}},
		{"form", "statement", "Требование сформулировано как утверждение.", []string{"declarative_assertion_or_directive"}},
		{"form", "acceptance_criterion", "Поля квитанции можно проверить.", []string{"acceptance_condition", "verifiable_outcome"}},
		{"content", "business_rule", "Правило определяет обработку очереди.", []string{"domain_policy"}},
	}
	digest := strings.Repeat("a", 64)
	model := "openvino-npu.assessment." + digest
	var calls, counts atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managerBinary != "" && r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{"profile": digest, "device": "NPU"})
			return
		}
		counting := r.URL.Path == "/v1/chat/token-count" || r.URL.Path == "/v1/count-tokens"
		var call int32
		if counting {
			call = counts.Add(1)
		} else {
			call = calls.Add(1)
		}
		if call > int32(len(expectedCalls)) {
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
			bound = (r.URL.Path == "/v1/generate" || r.URL.Path == "/v1/count-tokens") && request.Profile == digest
		} else {
			bound = (r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/v1/chat/token-count") && request.Model == model
			schema = request.ResponseFormat.JSONSchema.Schema
		}
		if err != nil || !bound || len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Error("unexpected model request", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		expected := expectedCalls[call-1]
		system, user := request.Messages[0].Content, request.Messages[1].Content
		var output struct{ Properties map[string]json.RawMessage }
		if !strings.Contains(system, `"axis":"`+expected.axis+`"`) || !strings.Contains(system, `"category":"`+expected.category+`"`) ||
			json.Unmarshal(schema, &output) != nil || len(output.Properties) != 4 || output.Properties["assessment"] == nil ||
			output.Properties["questions"] == nil || output.Properties["wording"] == nil || output.Properties["limitations"] == nil {
			t.Errorf("model request %d should assess %s category %s", call, expected.axis, expected.category)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, criterion := range expected.criteria {
			if !strings.Contains(system, criterion) || !strings.Contains(string(output.Properties["assessment"]), criterion) {
				t.Errorf("model request %d omitted %s", call, criterion)
			}
		}
		for _, other := range expectedCalls {
			if other.category == expected.category {
				continue
			}
			for _, criterion := range other.criteria {
				if strings.Contains(system, criterion) || strings.Contains(user, criterion) || strings.Contains(string(schema), criterion) {
					t.Errorf("model request %d included %s from category %s", call, criterion, other.category)
				}
			}
			if strings.Contains(system, other.reason) || strings.Contains(user, other.reason) {
				t.Errorf("model request %d included a previous assessment", call)
			}
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
		if counting {
			budget := map[string]any{"input_tokens": 1400, "max_input_tokens": 4096, "max_completion_tokens": 2048}
			if managerBinary != "" {
				budget["profile"], budget["device"] = digest, "NPU"
			} else {
				budget["model"], budget["exact"] = model, true
			}
			json.NewEncoder(w).Encode(budget)
			return
		}
		criteria := make([]string, 0, len(expected.criteria))
		for _, criterion := range expected.criteria {
			criteria = append(criteria, fmt.Sprintf(`%q:{"status":"present","evidence":%d,"note":"Признак явно указан."}`, criterion, call))
		}
		content := fmt.Sprintf(`{
			"assessment":{"verdict":"fits","reason":%q,"criteria":{%s}},
			"questions":[],"wording":"","limitations":"Оценено одно требование."
		}`, expected.reason, strings.Join(criteria, ","))
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
	openServer := func() *Server {
		t.Helper()
		generator, err := analysis.NewOpenAICompatible(base, model, "")
		if err != nil {
			t.Fatal(err)
		}
		server, err := NewConfigured(database, map[string]string{"token-a": "a", "token-b": "b"}, Config{
			Generator: generator, DefaultAnalysisThreshold: contracts.AnalysisConstraints().DefaultThreshold,
		})
		if err != nil {
			t.Fatal(err)
		}
		return server
	}
	s := openServer()
	defer func() { s.Close() }()
	_, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`)
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
		explainRequirementClassification(input:{projectId:"p", requirementId:"r", textVersion:1, revision:1, suggestionRevision:1, semanticType:%q, representationType:%q, locale:"ru"}) {
			status result { meaning { verdict criteria { evidence } } form { verdict criteria { evidence } } }
		}
	}`
	check := func(token, semantic, representation, meaningEvidence, formEvidence string, totalCalls int32) {
		t.Helper()
		res := summaryRequest(t, s, token, fmt.Sprintf(mutation, semantic, representation))
		if res["errors"] != nil {
			t.Fatal(res)
		}
		data, _ := res["data"].(map[string]any)
		got, _ := data["explainRequirementClassification"].(map[string]any)
		if calls.Load() != totalCalls || counts.Load() != totalCalls {
			t.Fatalf("%s / %s with %s: expected %d budget and inference requests, got %d and %d (%v)", semantic, representation, token, totalCalls, counts.Load(), calls.Load(), res)
		}
		if token == "token-b" {
			if got["status"] != "stale" || got["result"] != nil {
				t.Fatal("foreign workspace received an assessment", res)
			}
			return
		}
		result, _ := got["result"].(map[string]any)
		meaning, _ := result["meaning"].(map[string]any)
		form, _ := result["form"].(map[string]any)
		meaningCriteria, _ := json.Marshal(meaning["criteria"])
		formCriteria, _ := json.Marshal(form["criteria"])
		if got["status"] != "ready" || meaning["verdict"] != "fits" || form["verdict"] != "fits" ||
			!strings.Contains(string(meaningCriteria), `"evidence":"`+meaningEvidence+`"`) ||
			!strings.Contains(string(formCriteria), `"evidence":"`+formEvidence+`"`) {
			t.Fatalf("expected a grounded assessment for %s / %s: %v", semantic, representation, res)
		}
	}
	check("token-b", "functional_requirement", "statement", "", "", 0)
	check("token-a", "functional_requirement", "statement", firstPassage, secondPassage, 2)
	check("token-a", "functional_requirement", "statement", firstPassage, secondPassage, 2)
	check("token-a", "functional_requirement", "acceptance_criterion", firstPassage, thirdPassage, 3)
	check("token-a", "business_rule", "statement", fourthPassage, secondPassage, 4)
	check("token-b", "functional_requirement", "statement", "", "", 4)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openServer()
	check("token-b", "functional_requirement", "statement", "", "", 4)
	check("token-a", "business_rule", "acceptance_criterion", fourthPassage, thirdPassage, 4)
	check("token-a", "functional_requirement", "statement", firstPassage, secondPassage, 4)
}

func (*explanationStub) ExplanationProfile() string { return "test-profile" }
func (g *explanationStub) ExplainClassificationAxes(_ context.Context, _ explanation.Source, target explanation.Target, axes []explanation.Axis) (map[explanation.Axis]explanation.AxisResult, error) {
	g.calls++
	results := make(map[explanation.Axis]explanation.AxisResult, len(axes))
	for _, axis := range axes {
		assessment := explanation.Assessment{Verdict: "partial", Reason: target.Representation}
		if axis == explanation.MeaningAxis {
			assessment = explanation.Assessment{Verdict: "fits", Reason: target.Semantic}
		}
		results[axis] = explanation.AxisResult{Assessment: assessment, Limitations: "One record."}
	}
	return results, nil
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
