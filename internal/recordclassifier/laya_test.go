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
	for _, change := range []string{"valid", "repin", "invalid_scores", "training_context", "reserved"} {
		t.Run(change, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var response any
				switch r.URL.Path {
				case "/api/tags":
					digest := Digest
					if change == "repin" && calls > 0 {
						digest = strings.Repeat("a", 64)
					}
					response = map[string]any{"models": []any{map[string]string{"name": Model, "digest": digest}}}
				case "/api/version":
					response = map[string]string{"version": "0.40.16"}
				case "/api/show":
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
					if req.State != "Дано: пустая очередь." || req.Model != Model || len(req.Questions) != 1 || len(question.Criteria) < 8 {
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
					if change == "training_context" {
						tokens = 1025
					}
					response = map[string]any{"model": Model, "usage": map[string]int{"input_tokens": tokens, "output_tokens": 1}, "answers": map[string]any{"classification": map[string]any{"type": "choice", "choice": "a", "confidence": 1, "probabilities": scores}}}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			classifier, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			source := "Дано: пустая очередь."
			if change == "reserved" {
				source += " <mask>"
			}
			pred, err := classifier.Classify(context.Background(), source)
			if change == "repin" || change == "invalid_scores" {
				if err == nil {
					t.Fatal("invalid response was accepted")
				}
				return
			}
			if err != nil || pred.Status != "needs_review" || pred.Semantic != "unknown" || pred.Representation != "unknown" {
				t.Fatalf("prediction %+v err %v", pred, err)
			}
			if change == "valid" && (calls != 2 || len(pred.Alternatives) != 17 || pred.Reason != "uncalibrated") {
				t.Fatal(pred, calls)
			}
			if change == "training_context" && len(pred.Alternatives) > 0 {
				t.Fatal("unsupported input yielded scores")
			}
			if change == "reserved" && calls != 0 {
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
	adapter, err := New("http://127.0.0.1:11434")
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
	if err != nil || pred.Status != "needs_review" || pred.Reason != "uncalibrated" || len(pred.Alternatives) != 17 {
		t.Fatalf("review status %v, alternatives %v, error %v", pred.Status, len(pred.Alternatives), err)
	}
}
