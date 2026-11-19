package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/glossary"
)

func TestSuggestDefinitionFromRequirement(t *testing.T) {
	for _, tc := range []struct{ name, before, abbreviation, after, nameHint, definition, evidence string }{
		{"explicit expansion", "📝 Генеральный подрядчик (", "ГП", ") принимает работы.", "Генеральный подрядчик", "Генеральный подрядчик, принимающий работы.", "Генеральный подрядчик (ГП) принимает работы."},
		{"role in another sentence", "The approval screen identifies the ", "RA", ". Only this role can release the signed report.", "", "The role authorized to release the signed report.", "Only this role can release the signed report."},
		{"mention alone", "Передать в ", "АБ", ".", "Архивное бюро", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tags, chats int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/tags":
					tags++
					summaryTags(w, summaryDigest)
				case "/api/chat":
					chats++
					var request struct {
						Model    string
						Messages []struct{ Role, Content string }
						Format   struct{ Properties map[string]json.RawMessage }
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if request.Model != summaryModel || len(request.Messages) != 2 || request.Messages[1].Role != "user" || request.Format.Properties["definition"] == nil || request.Format.Properties["evidence"] == nil {
						t.Errorf("unexpected request: %+v", request)
						return
					}
					var source struct{ Before, Abbreviation, After, NameHint string }
					if err := json.Unmarshal([]byte(request.Messages[1].Content), &source); err != nil || source.Before != tc.before || source.Abbreviation != tc.abbreviation || source.After != tc.after || source.NameHint != tc.nameHint {
						t.Errorf("source or highlighted occurrence changed: %+v, %v", source, err)
					}
					content, _ := json.Marshal(map[string]string{"definition": tc.definition, "evidence": tc.evidence})
					json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]string{"role": "assistant", "content": string(content)}})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer server.Close()
			g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
			if err != nil {
				t.Fatal(err)
			}
			start := utf8.RuneCountInString(tc.before)
			var generator glossary.DefinitionGenerator = g
			got, err := generator.SuggestDefinition(context.Background(), glossary.DefinitionRequest{Text: tc.before + tc.abbreviation + tc.after, Start: start, End: start + utf8.RuneCountInString(tc.abbreviation), Abbreviation: tc.abbreviation, Name: tc.nameHint})
			if err != nil || got != tc.definition || tags != 2 || chats != 1 {
				t.Errorf("definition=%q err=%v tags=%d chats=%d", got, err, tags, chats)
			}
		})
	}
}

func TestDecodeDefinitionRejectsMalformedOrUnsupportedProposal(t *testing.T) {
	source := "QA checks products."
	for _, raw := range []string{
		`{}`, `{"definition":null,"evidence":""}`, `{"definition":" " ,"evidence":""}`,
		`{"definition":"","evidence":"QA"}`, `{"definition":"","evidence":"","extra":true}`,
		`{"definition":"","evidence":""}{}`, `{"definition":"Checks products.","evidence":""}`,
		`{"definition":"Checks products.","evidence":"unrelated excerpt"}`,
		`{"definition":"Checks\u0000products.","evidence":"QA checks products."}`,
		`{"definition":"` + strings.Repeat("a", contracts.GlossaryRules().DefinitionMax+1) + `","evidence":"QA checks products."}`,
	} {
		if got, err := decodeDefinition(raw, source); err == nil || got != "" {
			t.Errorf("accepted malformed result: %q, got %q", raw, got)
		}
	}
}

func TestSuggestDefinitionRejectsWrongLanguage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		content := `{"definition":"The party that signs the contract.","evidence":"ГП подписывает договор."}`
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]string{"role": "assistant", "content": content}})
	}))
	defer server.Close()
	g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.SuggestDefinition(context.Background(), glossary.DefinitionRequest{Text: "ГП подписывает договор.", Abbreviation: "ГП", Start: 0, End: 2})
	if err == nil || got != "" {
		t.Fatalf("accepted English proposal for a Russian source: %q, %v", got, err)
	}
}
