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
	const text = firstPassage + " Кассир выбирает операцию за текущую смену. " +
		"На бумаге указываются дата продажи, номер операции, итоговая сумма, валюта и способ оплаты. " +
		"Если принтер занят, система показывает состояние задания и держит его в очереди. " +
		"Кассир использует квитанцию для сверки операции с отчётом по смене."
	digest := strings.Repeat("a", 64)
	model := "openvino-npu.assessment." + digest
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managerBinary != "" && r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{"profile": digest, "device": "NPU"})
			return
		}
		calls.Add(1)
		var request struct {
			Model    string
			Profile  string
			Messages []struct{ Content string }
		}
		err := json.NewDecoder(r.Body).Decode(&request)
		var bound bool
		if managerBinary != "" {
			bound = r.URL.Path == "/v1/generate" && request.Profile == digest
		} else {
			bound = r.URL.Path == "/v1/chat/completions" && request.Model == model
		}
		if err != nil || !bound || len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, firstPassage) ||
			!strings.Contains(request.Messages[1].Content, `"savedDecision":null`) || strings.Contains(request.Messages[1].Content, `"alternatives"`) {
			t.Error("unexpected model request", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		const content = `{
			"content":{"verdict":"fits","reason":"Печать квитанции описывает работу системы.","criteria":{"system_behavior":{"status":"present","evidence":1,"note":"Действие системы явно указано."}}},
			"form":{"verdict":"fits","reason":"Требование сформулировано как утверждение.","criteria":{"declarative_assertion_or_directive":{"status":"present","evidence":1,"note":"Утверждение описывает действие."}}},
			"questions":[],"wording":"","limitations":"Оценено одно требование."
		}`
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
			status result { meaning { verdict criteria { evidence } } form { verdict } }
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
		encoded, _ := json.Marshal(got)
		if got["status"] != "ready" || !strings.Contains(string(encoded), `"evidence":"`+firstPassage+`"`) || calls.Load() != 1 {
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
