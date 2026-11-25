package recordclassifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/VolkovDani/project-creator/internal/classification"
)

const testManagedID = "ollama.laya-review." + d3Digest
const testManagedSource = "Дано: пустая очередь."

type testChoiceQuestion struct {
	Type, Instructions string
	Criteria           map[string]string
}

func testDecisionAnswer(q testChoiceQuestion) map[string]any {
	probabilities := make(map[string]float64, len(q.Criteria))
	for key := range q.Criteria {
		probabilities[key] = 0
	}
	probabilities["a"] = 1
	return map[string]any{"type": "choice", "choice": "a", "confidence": 1, "probabilities": probabilities}
}

func TestManagedDecisionMatchesDirectReview(t *testing.T) {
	upstream := httptest.NewServer(testOllamaDecision(t))
	defer upstream.Close()
	direct, err := New(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	var questions map[string]struct{ Wire testChoiceQuestion }
	if err := json.Unmarshal(flatQuestions, &questions); err != nil {
		t.Fatal(err)
	}
	for _, test := range []string{
		"valid", "wrong_model", "wrong_digest", "invalid_scores", "invalid_usage", "conflicting_reason", "unknown_reason",
		"reserved_token", "input_limit", "outside_training_context", "partial_result",
	} {
		t.Run(test, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				var req struct {
					Model, Digest, State string
					Question             testChoiceQuestion
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				axis := "semantic"
				if call == 2 {
					axis = "representation"
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/decisions" || req.Model != testManagedID ||
					req.Digest != Digest || req.State != testManagedSource || !reflect.DeepEqual(req.Question, questions[axis].Wire) {
					t.Errorf("unexpected managed request %s %s %+v", r.Method, r.URL.Path, req)
				}
				response := map[string]any{"model": testManagedID, "digest": Digest}
				ans := testDecisionAnswer(req.Question)
				switch test {
				case "wrong_model":
					response["model"] = Model
				case "wrong_digest":
					response["digest"] = d3Digest
				case "invalid_scores":
					ans["probabilities"].(map[string]float64)["a"] = 0.4
				case "conflicting_reason":
					response["reason"] = "input_limit"
				case "reserved_token", "input_limit", "outside_training_context", "unknown_reason":
					response["reason"] = test
					json.NewEncoder(w).Encode(response)
					return
				case "partial_result":
					if call == 2 {
						response["reason"] = "outside_training_context"
						json.NewEncoder(w).Encode(response)
						return
					}
				}
				response["answer"] = ans
				if test != "invalid_usage" {
					response["usage"] = map[string]int{"input_tokens": 58, "output_tokens": 1}
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			managed, err := NewManaged(server.URL+"/v1", testManagedID, Model, "sha256:"+Digest)
			if err != nil {
				t.Fatal(err)
			}
			if managed.Model() != direct.Model() || managed.Digest() != direct.Digest() ||
				managed.QuestionVersion() != direct.QuestionVersion() || managed.Profile() == direct.Profile() {
				t.Fatal("managed decision adapter did not preserve artifact and taxonomy identity with distinct provenance")
			}
			got, err := managed.Classify(context.Background(), testManagedSource)
			switch test {
			case "wrong_model", "wrong_digest", "invalid_scores", "invalid_usage", "conflicting_reason", "unknown_reason":
				if err == nil || calls.Load() != 1 {
					t.Fatalf("accepted invalid response: %+v, %v, %d calls", got, err, calls.Load())
				}
			case "reserved_token", "input_limit", "outside_training_context", "partial_result":
				want := classification.Prediction{Status: "needs_review", Semantic: "unknown", Representation: "unknown", Reason: test}
				if test == "partial_result" {
					want.Reason = "outside_training_context"
					good, err := direct.Classify(context.Background(), testManagedSource)
					if err != nil {
						t.Fatal(err)
					}
					want.Alternatives = good.Alternatives[:9]
				}
				if err != nil || calls.Load() != 2 || !reflect.DeepEqual(got, want) {
					t.Fatalf("got %+v, %v, %d calls, want %+v", got, err, calls.Load(), want)
				}
			default:
				want, directErr := direct.Classify(context.Background(), testManagedSource)
				if err != nil || directErr != nil || calls.Load() != 2 || !reflect.DeepEqual(got, want) || len(got.Alternatives) != 17 {
					t.Fatalf("got %+v, %v, %d calls, want %+v, %v", got, err, calls.Load(), want, directErr)
				}
			}
		})
	}
}

func testOllamaDecision(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response any
		switch r.URL.Path {
		case "/api/tags":
			response = map[string]any{"models": []map[string]string{{"name": Model, "digest": Digest}}}
		case "/api/version":
			response = map[string]string{"version": "0.40.16"}
		case "/api/show":
			response = map[string]any{"details": map[string]string{"runner": "llamacpp", "format": "gguf"}, "model_info": map[string]any{"modern-bert.decision.type": "laya", "modern-bert.decision.max_head_tokens": 256}, "capabilities": []string{"decision"}}
		case "/v1/systemone":
			var req struct {
				Model, State string
				Questions    map[string]testChoiceQuestion
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model != Model || req.State != testManagedSource || len(req.Questions) != 1 {
				t.Errorf("unexpected Ollama decision request %+v: %v", req, err)
			}
			answers := make(map[string]any, len(req.Questions))
			for key, q := range req.Questions {
				answers[key] = testDecisionAnswer(q)
			}
			response = map[string]any{"model": Model, "usage": map[string]int{"input_tokens": 58, "output_tokens": 1}, "answers": answers}
		default:
			t.Errorf("unexpected Ollama path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(response)
	})
}

func TestManagedDecisionAcceptsOnlyExplicitLocalV1EndpointAndImmutableProfile(t *testing.T) {
	for _, url := range []string{
		"", "http://localhost:8317", "http://models-configurator:8317", "http://models-configurator:11434/v1",
		"https://127.0.0.1:8317/v1", "http://user@localhost:8317/v1", "http://example.net/v1",
		"http://127.0.0.1:8317/proxy/v1", "http://127.0.0.1:8317/%76%31", "http://localhost:8317/v1?insecure=1",
		"http://localhost:8317/v1#", "http://ollama:11434/v1",
	} {
		if _, err := NewManaged(url, testManagedID, Model, Digest); err == nil {
			t.Errorf("accepted managed URL %q", url)
		}
	}
	for _, id := range []string{"", "ollama.laya-review", "ollama.laya-review.1234", "openai-compatible.laya-review." + Digest, "ollama.." + Digest, "ollama.laya-review." + strings.ToUpper(Digest)} {
		if _, err := NewManaged("http://models-configurator:8317/v1", id, Model, Digest); err == nil {
			t.Errorf("accepted managed ID %q", id)
		}
	}
	for _, url := range []string{"http://models-configurator:8317/v1", "http://127.0.0.1:1337/v1/", "http://[::1]:8317/v1"} {
		if _, err := NewManaged(url, testManagedID, Model, Digest); err != nil {
			t.Errorf("rejected local manager URL %q: %v", url, err)
		}
	}
}

func TestManagedDecisionDoesNotFollowRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("managed decision was redirected")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/decisions", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	adapter, err := NewManaged(redirect.URL+"/v1", testManagedID, Model, Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Classify(context.Background(), testManagedSource); err == nil {
		t.Fatal("redirect was accepted")
	}
}

func TestManagedDecisionDelegatesUpstreamBodyLimit(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]string{"model": testManagedID, "digest": Digest, "reason": "input_limit"})
	}))
	defer server.Close()
	managed, err := NewManaged(server.URL+"/v1", testManagedID, Model, Digest)
	if err != nil {
		t.Fatal(err)
	}
	// JSON escaping can expand a small record past Ollama's envelope limit.
	got, err := managed.Classify(t.Context(), strings.Repeat("<", maxBodyBytes/6))
	if err != nil || got.Reason != "input_limit" || calls.Load() != 2 {
		t.Fatalf("manager did not evaluate the upstream body limit: %+v, %v, %d calls", got, err, calls.Load())
	}
}
