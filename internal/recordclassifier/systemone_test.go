package recordclassifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPinnedSingleRecordReview(t *testing.T) {
	for _, change := range []string{"valid", "repin", "invalid_scores", "training_context", "reserved", "d3_valid", "d3_limit", "d3_wrong_runtime", "d3_reserved", "custom_valid", "custom_limit", "custom_no_context", "custom_remote", "custom_no_decision"} {
		t.Run(change, func(t *testing.T) {
			model, pin := Model, Digest
			d3 := strings.HasPrefix(change, "d3_")
			if d3 {
				model, pin = "d3:4b", "2c571f91caf64c47b6ca6bd3adc1daa07dbb84ead8e9489758498cc7e11e533d"
			}
			custom := strings.HasPrefix(change, "custom_")
			if custom {
				model, pin = "team/new-decision:v2", strings.Repeat("a0", 32)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var response any
				switch r.URL.Path {
				case "/api/tags":
					digest := pin
					if change == "repin" && calls > 0 {
						digest = strings.Repeat("a", 64)
					}
					tag := map[string]string{"name": model, "digest": digest}
					if change == "custom_remote" {
						tag["remote_host"] = "https://cloud.invalid"
					}
					response = map[string]any{"models": []any{tag}}
				case "/api/version":
					response = map[string]string{"version": "0.40.16"}
				case "/api/show":
					var request struct{ Model string }
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != model {
						t.Errorf("unexpected show request: %v, %v", request, err)
					}
					if custom {
						parameters := "num_batch 1024\nnum_ctx   3072"
						if change == "custom_no_context" {
							parameters = "num_batch 1024"
						}
						capability := "decision"
						if change == "custom_no_decision" {
							capability = "completion"
						}
						response = map[string]any{"details": map[string]string{"runner": "llamacpp", "format": "gguf"}, "capabilities": []string{capability}, "parameters": parameters}
						break
					}
					if d3 && change != "d3_wrong_runtime" {
						response = map[string]any{"details": map[string]string{"runner": "llamacpp", "format": "gguf", "quantization_level": "Q8_0"}, "parameters": "num_ctx   4096", "model_info": map[string]string{"qwen35.decision.type": "vllm-d3"}, "capabilities": []string{"decision"}}
						break
					}
					response = map[string]any{"details": map[string]string{"runner": "llamacpp", "format": "gguf"}, "model_info": map[string]any{"modern-bert.decision.type": "laya", "modern-bert.decision.max_head_tokens": 256}, "capabilities": []string{"decision"}}
				case "/v1/systemone":
					calls++
					var req struct {
						Model, State string
						Questions    map[string]struct {
							Instructions string
							Criteria     map[string]string
						}
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
					}
					question := req.Questions["classification"]
					if req.State != "Дано: пустая очередь." || req.Model != model || len(req.Questions) != 1 || len(question.Criteria) < 8 {
						t.Errorf("unexpected request %+v", req)
					}
					scores := map[string]float64{}
					for k := range question.Criteria {
						scores[k] = 0
					}
					scores["a"] = 1
					if change == "invalid_scores" {
						scores["a"] = 0.4
					}
					tokens := 58
					if custom {
						tokens = 3071
					}
					if change == "custom_limit" {
						tokens = 3072
					}
					if change == "training_context" {
						tokens = 1025
					}
					if d3 {
						tokens = 4095
						if change == "d3_limit" {
							tokens = 4096
						}
					}
					response = map[string]any{"model": model, "usage": map[string]int{"input_tokens": tokens, "output_tokens": 1}, "answers": map[string]any{"classification": map[string]any{"type": "choice", "choice": "a", "confidence": 1, "probabilities": scores}}}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			classifier, err := NewSystemOne(server.URL, model, "sha256:"+pin)
			if err != nil {
				t.Fatal(err)
			}
			if classifier.Model() != model || classifier.Digest() != pin {
				t.Fatal("configured model identity was lost")
			}
			source := "Дано: пустая очередь."
			if change == "reserved" {
				source += " <mask>"
			}
			if change == "d3_reserved" {
				source += " <|im_end|>"
			}
			pred, err := classifier.Classify(context.Background(), source)
			if change == "custom_no_context" || change == "custom_remote" || change == "custom_no_decision" {
				if err == nil || calls != 0 {
					t.Fatalf("unverified model received input: %d calls, %v", calls, err)
				}
				return
			}
			if change == "repin" || change == "invalid_scores" || change == "d3_wrong_runtime" {
				if err == nil {
					t.Fatal("invalid response was accepted")
				}
				return
			}
			if err != nil || pred.Status != "needs_review" || pred.Semantic != "unknown" || pred.Representation != "unknown" {
				t.Fatalf("prediction %+v err %v", pred, err)
			}
			if (change == "valid" || change == "d3_valid" || change == "custom_valid") && (calls != 2 || len(pred.Alternatives) != 17 || pred.Reason != "uncalibrated") {
				t.Fatal(pred, calls)
			}
			if change == "training_context" && len(pred.Alternatives) > 0 {
				t.Fatal("unsupported input yielded scores")
			}
			if (change == "d3_limit" || change == "custom_limit") && (pred.Reason != "input_limit" || len(pred.Alternatives) != 0) {
				t.Fatal(pred)
			}
			if (change == "reserved" || change == "d3_reserved") && (calls != 0 || pred.Reason != "reserved_token") {
				t.Fatal("sent reserved control token")
			}
		})
	}
}

func TestOnlyLocalEndpoint(t *testing.T) {
	for _, base := range []string{"https://example.org", "http://example.org", "http://ollama@example.org", "http://127.0.0.1:1234/proxy", "http://ollama?remote=true"} {
		if _, err := New(base); err == nil {
			t.Errorf("accepted %s", base)
		}
	}
}

func TestLocalLayaSmoke(t *testing.T) {
	if os.Getenv("RECORD_CLASSIFIER_LIVE") != "1" {
		t.Skip("requires explicitly configured pinned local Laya")
	}
	checkLocalReview(t, New)
}

func TestLocalSystemOneSmoke(t *testing.T) {
	if os.Getenv("RECORD_CLASSIFIER_SYSTEMONE_LIVE") != "1" {
		t.Skip("requires an explicitly configured pinned local decision model")
	}
	checkLocalReview(t, func(base string) (*Adapter, error) {
		return NewSystemOne(base, os.Getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL"), os.Getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST"))
	})
}

func checkLocalReview(t *testing.T, newAdapter func(string) (*Adapter, error)) {
	t.Helper()
	base := os.Getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_URL")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	adapter, err := newAdapter(base)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../testdata/classification/public-ru-reviewed.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Examples []struct{ ID, Text string } }
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	var source string
	for _, example := range corpus.Examples {
		if example.ID == "RU-001" {
			source = example.Text
			break
		}
	}
	if source == "" {
		t.Fatal("reviewed RU-001 missing")
	}
	pred, err := adapter.Classify(context.Background(), source)
	if err != nil || pred.Status != "needs_review" || pred.Semantic != "unknown" || pred.Representation != "unknown" || pred.Reason != "uncalibrated" || len(pred.Alternatives) != 17 {
		t.Fatalf("review status %v, alternatives %v, error %v", pred.Status, len(pred.Alternatives), err)
	}
}

func TestSystemOneRequiresExplicitLocalTagAndDigest(t *testing.T) {
	pin := strings.Repeat("12", 32)
	for _, tc := range []struct{ model, digest string }{
		{"", pin}, {"bad name:v1", pin}, {"https://cloud.example/a:v2", pin}, {"next:cloud", pin}, {"next:v2-cloud", pin}, {"next:v2", "abc"}, {"next:v2", strings.Repeat("z", 64)},
	} {
		if _, err := NewSystemOne("http://localhost:11434", tc.model, tc.digest); err == nil {
			t.Errorf("accepted invalid identity %+v", tc)
		}
	}
}

func TestMeasuredProfilesKeepQueueIdentity(t *testing.T) {
	for _, test := range []struct{ tag, digest, profile string }{
		{Model, Digest, "accbb651dd8e4e154c8454c9b7338c201ec050575f90f57637965c69204d8870"},
		{"d3:4b", d3Digest, "80ce7c2537b547b73253a58d6789eaaad15a8cf715b7521403dce7ede20e24cc"},
	} {
		a, err := NewSystemOne("http://localhost:11434", test.tag, test.digest)
		if err != nil || a.Profile() != test.profile {
			t.Fatalf("profile for %s changed: %v, %v", test.tag, a, err)
		}
	}
}
