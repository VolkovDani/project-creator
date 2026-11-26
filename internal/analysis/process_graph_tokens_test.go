package analysis

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func tokenPlanFixture() processgraph.Request {
	req := processgraph.Request{Anchor: processgraph.Anchor{Name: "request"}}
	for i := range 24 {
		req.Sources = append(req.Sources, processgraph.Source{
			ID: fmt.Sprint(i), Version: 1, Text: fmt.Sprintf("Review request %d. ", i) + strings.Repeat("Check the remaining recorded conditions. ", 10),
		})
	}
	return req
}

type tokenPlanChat struct {
	Model               string              `json:"model"`
	Messages            []map[string]string `json:"messages"`
	ResponseFormat      json.RawMessage     `json:"response_format"`
	MaxCompletionTokens int                 `json:"max_completion_tokens"`
}

func TestProcessGraphPlansAgainstTheConfiguredChatTokenBudget(t *testing.T) {
	for _, provider := range []string{"openvino-npu", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			model := provider + ".test." + strings.Repeat("a", 64)
			var mu sync.Mutex
			budget := 1050
			measured := map[string]int{}
			var generated int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body tokenPlanChat
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != model || len(body.Messages) != 2 ||
					body.MaxCompletionTokens != chatCompletionTokens || len(body.ResponseFormat) == 0 {
					t.Errorf("unexpected %s request: %+v, %v", r.URL.Path, body, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				key := body.Messages[0]["content"] + "\n" + body.Messages[1]["content"] + "\n" + string(body.ResponseFormat)
				switch r.URL.Path {
				case "/v1/chat/token-count":
					tokens := len([]rune(key)) / 4
					measured[key] = tokens
					_ = json.NewEncoder(w).Encode(map[string]any{
						"model": model, "input_tokens": tokens, "max_input_tokens": budget,
						"max_completion_tokens": 512, "exact": provider == "openvino-npu",
					})
				case "/v1/chat/completions":
					if tokens := measured[key]; tokens <= 0 || tokens > budget {
						t.Errorf("generation sent a different or over-budget prompt: counted=%d, budget=%d", tokens, budget)
					}
					generated++
					_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"r\":[]}"}}]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			g, err := NewSummaryOpenAICompatible(server.URL+"/v1", model, "", "en")
			if err != nil {
				t.Fatal(err)
			}
			req := tokenPlanFixture()
			largerBatch := func() processgraph.BatchPlan {
				t.Helper()
				plan, err := g.PlanProcessGraphWithContext(t.Context(), req)
				if err != nil || plan.Partial {
					t.Fatalf("plan: %+v, %v", plan, err)
				}
				return plan
			}
			small := largerBatch()
			mu.Lock()
			budget = 6000
			mu.Unlock()
			large := largerBatch()
			if len(small.Batches) < 2 || len(large.Batches) != 1 || len(large.Batches[0].Sources) != len(req.Sources) {
				t.Fatalf("larger window did not reduce planned steps: %d → %d", len(small.Batches), len(large.Batches))
			}
			mu.Lock()
			callsBeforeGeneration := generated
			mu.Unlock()
			if callsBeforeGeneration != 0 {
				t.Fatal("planning ran inference")
			}
			answer, err := g.GenerateProcessGraphBatch(t.Context(), large.Batches[0])
			if err != nil || answer.UsedSources != len(req.Sources) {
				t.Fatalf("the measured batch was rejected or truncated: %+v, %v", answer, err)
			}
		})
	}
}

func TestProcessGraphPreflightCountsAndSendsWholeRequirements(t *testing.T) {
	model := "openvino-npu.test." + strings.Repeat("a", 64)
	req := processgraph.Request{
		Anchor: processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{
			{ID: "r1", Version: 1, Text: "The queue opens.\n"},
			{ID: "r2", Version: 3, Text: " 🧾 The queue waits.\n\nThe queue closes.\n\t"},
		},
	}
	generated := make(chan []string, len(req.Sources))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body tokenPlanChat
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
			t.Errorf("invalid model request: %+v, %v", body, err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var data struct{ Sources []graphSource }
		if err := json.Unmarshal([]byte(body.Messages[1]["content"]), &data); err != nil {
			t.Errorf("invalid prompt: %v", err)
			http.Error(w, "invalid prompt", http.StatusBadRequest)
			return
		}
		var texts []string
		var tokens, partNumber int
		for i, source := range data.Sources {
			if source.Number != i+1 {
				t.Errorf("incorrect requirement number: %+v", source)
			}
			var text strings.Builder
			for _, part := range source.Parts {
				partNumber++
				if part.Number != partNumber {
					t.Errorf("incorrect citation number: %+v", part)
				}
				text.WriteString(part.Text)
			}
			value := text.String()
			switch value {
			case req.Sources[0].Text:
			case req.Sources[1].Text:
				if len(source.Parts) != 2 {
					t.Errorf("expected two separately citable passages in one requirement: %+v", source)
				}
			default:
				t.Errorf("%s received part of a requirement: %q", r.URL.Path, value)
			}
			tokens += len(value)
			texts = append(texts, value)
		}
		switch r.URL.Path {
		case "/v1/chat/token-count":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": model, "input_tokens": tokens, "max_input_tokens": len(req.Sources[1].Text),
				"max_completion_tokens": 512, "exact": true,
			})
		case "/v1/chat/completions":
			select {
			case generated <- texts:
			default:
				t.Error("too many model calls")
			}
			_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"r\":[]}"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	g, err := NewSummaryOpenAICompatible(server.URL+"/v1", model, "", "en")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := g.PlanProcessGraphWithContext(t.Context(), req)
	if err != nil || plan.Partial || len(plan.Batches) != len(req.Sources) {
		t.Fatalf("expected two complete batches: %+v, %v", plan, err)
	}
	for i, batch := range plan.Batches {
		response, err := g.GenerateProcessGraphBatch(t.Context(), batch)
		if err != nil || response.Partial || response.UsedSources != 1 {
			t.Fatalf("expected one full requirement in completion: %+v, %v", response, err)
		}
		select {
		case actual := <-generated:
			if !slices.Equal(actual, []string{req.Sources[i].Text}) {
				t.Errorf("generated batch %d should use the complete source: %q", i, actual)
			}
		default:
			t.Fatalf("batch %d did not call the model", i)
		}
	}
}

func TestProcessGraphTokenPreflightFallsBackWithOlderManager(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/token-count" {
			t.Errorf("unexpected network request during planning: %s", r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	model := "openvino-npu.test." + strings.Repeat("b", 64)
	g, err := NewSummaryOpenAICompatible(server.URL+"/v1", model, "", "en")
	if err != nil {
		t.Fatal(err)
	}
	req := tokenPlanFixture()
	legacy, err := g.PlanProcessGraph(req)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := g.PlanProcessGraphWithContext(t.Context(), req)
	if err != nil || len(actual.Batches) != len(legacy.Batches) || actual.Partial != legacy.Partial {
		t.Fatalf("failed to retain the legacy plan: %+v, %v", actual, err)
	}
}
