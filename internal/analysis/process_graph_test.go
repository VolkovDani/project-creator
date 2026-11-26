package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphUsesConfiguredChatAndOriginalText(t *testing.T) {
	for _, backend := range []string{"ollama", "compatible"} {
		t.Run(backend, func(t *testing.T) {
			const quote = "После отправки заявки начинается проверка."
			const source = "# Условие\n" + quote
			const answer = `{"r":[{"a":"Отправить","b":"Проверить","c":"","k":"triggers","p":[2],"x":"action","y":"action"}]}`
			tags, chats := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" && backend == "ollama" {
					tags++
					summaryTags(w, summaryDigest)
					return
				}
				if backend == "ollama" && r.URL.Path != "/api/chat" ||
					backend == "compatible" && r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected generation path %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				chats++
				var payload struct {
					Messages []map[string]string
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.Messages) != 2 {
					t.Errorf("invalid prompt payload: %v", err)
					return
				}
				system, user := payload.Messages[0], payload.Messages[1]
				if system["role"] != "system" || (!strings.Contains(system["content"], "untrusted") && !strings.Contains(system["content"], "недоверенные")) ||
					user["role"] != "user" || strings.Contains(user["content"], "private-requirement-id") {
					t.Errorf("unexpected prompt: %v", payload.Messages)
				}
				var input struct {
					Anchor  processgraph.Anchor
					Sources []struct {
						Number int
						Parts  []struct {
							Number int `json:"p"`
							Text   string
						}
					}
				}
				if err := json.Unmarshal([]byte(user["content"]), &input); err != nil ||
					input.Anchor.Name != "Заявка" || input.Anchor.Definition != "Проектное понятие" ||
					len(input.Sources) != 1 || input.Sources[0].Number != 1 || len(input.Sources[0].Parts) != 2 || input.Sources[0].Parts[1].Number != 2 ||
					input.Sources[0].Parts[0].Text+input.Sources[0].Parts[1].Text != source {
					t.Errorf("request did not preserve the selected source: %+v, %v", input, err)
				}
				message := map[string]any{"role": "assistant", "content": answer}
				switch backend {
				case "ollama":
					json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": message})
				case "compatible":
					json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": message}}})
				}
			}))
			defer server.Close()

			var g processgraph.Generator
			var err error
			if backend == "ollama" {
				g, err = NewSummaryOllama(server.URL, summaryModel, summaryDigest)
			} else {
				g, err = NewSummaryOpenAICompatible(server.URL+"/v1", summaryModel, "", "")
			}
			if err != nil {
				t.Fatal(err)
			}
			draft, err := processgraph.Build(t.Context(), g, processgraph.Request{
				Anchor: processgraph.Anchor{Name: "Заявка", Definition: "Проектное понятие"},
				Sources: []processgraph.Source{
					{ID: "private-requirement-id", Version: 7, Text: source},
					{ID: "too-long", Version: 1, Text: strings.Repeat("🧪", 10000)},
				},
			})
			if err != nil || !draft.Partial || draft.SourceCount != 1 || len(draft.Edges) != 1 ||
				draft.Edges[0].Evidence[0].Quote != quote {
				t.Fatalf("expected one cited edge with partial input coverage: %+v, %v", draft, err)
			}
			if chats != 1 || (backend == "ollama" && tags != 2) {
				t.Fatalf("chats=%d provider verifications=%d", chats, tags)
			}
		})
	}
}

func TestProcessGraphRejectsMalformedCompletion(t *testing.T) {
	s := &chatSummary{
		verify: context.Context.Err,
		complete: func(context.Context, []map[string]string, map[string]any) (string, error) {
			return `{"r":null}`, nil
		},
	}
	_, err := s.GenerateProcessGraph(t.Context(), processgraph.Request{
		Anchor:  processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{{ID: "one", Version: 1, Text: "The queue is empty."}},
	})
	if err == nil {
		t.Fatal("expected invalid model response")
	}
}

func TestManagedNPUProcessGraphRetriesWithSmallerCitedContext(t *testing.T) {
	const quote = "Поступление заявки запускает проверку."
	var counts []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sources := processGraphTestSources(t, r)
		counts = append(counts, len(sources))
		if len(sources) > 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"message":"model provider failed","code":"NPU_UNAVAILABLE"}}`))
			return
		}
		processGraphTestCompletion(w, `{"r":[{"a":"Поступление заявки","b":"Проверить","c":"","k":"triggers","p":[2],"x":"event","y":"action"}]}`)
	}))
	defer server.Close()
	g, err := NewSummaryOpenAICompatible(server.URL+"/v1", "openvino-npu.fixture."+strings.Repeat("a", 64), "", "ru")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := processgraph.Build(t.Context(), g, processgraph.Request{
		Anchor: processgraph.Anchor{Name: "Заявка"},
		Sources: []processgraph.Source{
			{ID: "first", Version: 1, Text: "Заявка поступила."},
			{ID: "second", Version: 7, Text: quote},
			{ID: "third", Version: 1, Text: "Заявка проверяется."},
			{ID: "fourth", Version: 1, Text: "Заявка сохранена."},
		},
	})
	if err != nil || !draft.Partial || draft.SourceCount != 2 || len(draft.Edges) != 1 ||
		draft.Edges[0].Evidence[0].RequirementID != "second" || draft.Edges[0].Evidence[0].Version != 7 ||
		draft.Edges[0].Evidence[0].Quote != quote {
		t.Fatalf("expected a cited partial result from a smaller context: %+v, %v", draft, err)
	}
	if len(counts) != 2 || counts[0] != 4 || counts[1] != 2 {
		t.Fatalf("source counts per request: %v", counts)
	}
}

func TestManagedNPUProcessGraphKeepsCompletePassagesFromLongRecord(t *testing.T) {
	const quote = "Поступление заявки запускает проверку."
	record := strings.Repeat(quote+"\n", 160)
	var sentParts []graphPart
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sources := processGraphTestSources(t, r)
		if len(sources) == 1 {
			sentParts = sources[0].Parts
		}
		processGraphTestCompletion(w, `{"r":[{"a":"Поступление заявки","b":"Проверить","c":"","k":"triggers","p":[1],"x":"event","y":"action"}]}`)
	}))
	defer server.Close()
	g, err := NewSummaryOpenAICompatible(server.URL+"/v1", "openvino-npu.fixture."+strings.Repeat("a", 64), "", "ru")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := processgraph.Build(t.Context(), g, processgraph.Request{
		Anchor:  processgraph.Anchor{Name: "Заявка"},
		Sources: []processgraph.Source{{ID: "long", Version: 1, Text: record}},
	})
	if err != nil || !draft.Partial || draft.SourceCount != 1 || len(draft.Edges) != 1 ||
		draft.Edges[0].Evidence[0].Quote != quote {
		t.Fatalf("expected a partial result with a whole original passage: %+v, %v", draft, err)
	}
	if len(sentParts) == 0 || len(sentParts) >= 160 || sentParts[0].Text != quote+"\n" {
		t.Fatalf("unexpected context: sent %d passages", len(sentParts))
	}
}

func processGraphTestSources(t *testing.T, r *http.Request) []graphSource {
	t.Helper()
	var payload struct{ Messages []map[string]string }
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.Messages) != 2 {
		t.Errorf("invalid request: %v", err)
		return nil
	}
	var input struct{ Sources []graphSource }
	if err := json.Unmarshal([]byte(payload.Messages[1]["content"]), &input); err != nil {
		t.Errorf("invalid graph source input: %v", err)
	}
	return input.Sources
}

func processGraphTestCompletion(w http.ResponseWriter, content string) {
	json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{
		"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content},
	}}})
}
