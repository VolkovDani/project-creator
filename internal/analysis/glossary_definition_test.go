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
		{"role in another sentence", "The approval screen identifies the ", "RA", ". Only this role can release the signed report.", "", "The role authorized to release the signed report.", "The approval screen identifies the RA. Only this role can release the signed report."},
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
					if request.Model != summaryModel || len(request.Messages) != 2 || request.Messages[1].Role != "user" || request.Format.Properties["definition"] == nil {
						t.Errorf("unexpected request: %+v", request)
						return
					}
					var source struct{ Before, Abbreviation, After, NameHint string }
					if chats == 1 {
						if err := json.Unmarshal([]byte(request.Messages[1].Content), &source); err != nil || source.Before != tc.before || source.Abbreviation != tc.abbreviation || source.After != tc.after || source.NameHint != tc.nameHint {
							t.Errorf("source or highlighted occurrence changed: %+v, %v", source, err)
						}
					}
					content, _ := json.Marshal(map[string]string{"definition": tc.definition, "evidence": tc.evidence})
					if chats == 2 {
						content, _ = json.Marshal(map[string]string{"definition": ""})
					}
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
			wantChats := 1
			if tc.definition == "" {
				wantChats = 2
			}
			if err != nil || got.Text != tc.definition || got.Hypothesis || tags != 2 || chats != wantChats {
				t.Errorf("definition=%+v err=%v tags=%d chats=%d", got, err, tags, chats)
			}
		})
	}
}

func TestDecodeDefinitionRejectsMalformedOrUnsupportedProposal(t *testing.T) {
	req := glossary.DefinitionRequest{Text: "QA checks products.", Abbreviation: "QA", Start: 0, End: 2}
	for _, raw := range []string{
		`{}`, `{"definition":null,"evidence":""}`, `{"definition":" " ,"evidence":""}`,
		`{"definition":"","evidence":"","extra":true}`,
		`{"definition":"","evidence":""}{}`,
		`{"definition":"Checks\u0000products.","evidence":"QA checks products."}`,
		`{"definition":"` + strings.Repeat("a", contracts.GlossaryRules().DefinitionMax+1) + `","evidence":"QA checks products."}`,
	} {
		if got, err := decodeGroundedDefinition(raw, req); err == nil || got != "" {
			t.Errorf("accepted malformed result: %q, got %+v", raw, got)
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
	if err == nil || got.Text != "" {
		t.Fatalf("accepted English proposal for a Russian source: %+v, %v", got, err)
	}
}

// A model may paraphrase the selected mention or quote a neighboring list item.
// Neither should turn a successfully served request into a transport failure.
func TestSuggestDefinitionWeakEvidence(t *testing.T) {
	const src = "Фильтры заявки:\nЗапись из ЖВК\nКод подразделения"
	const abbr = "ЖВК"
	start := utf8.RuneCountInString(src[:strings.Index(src, abbr)])
	g, err := NewSummaryOllama("http://localhost:11434", summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	g.verify = func(context.Context) error { return nil }
	calls := 0
	g.complete = func(_ context.Context, messages []map[string]string, schema map[string]any) (string, error) {
		calls++
		if len(messages) != 2 || strings.Contains(messages[1]["content"], "Код подразделения заявки.") {
			t.Fatal("unexpected replay of rejected proposal")
		}
		if calls == 1 {
			return `{"definition":"Код подразделения заявки.","evidence":"  Код подразделения"}`, nil
		}
		field := schema["properties"].(map[string]any)["definition"].(map[string]any)
		if field["pattern"] == nil {
			t.Fatal("Russian contextual draft missing alphabet constraint")
		}
		if strings.Contains(messages[1]["content"], "Код подразделения") {
			t.Fatal("neighboring list item included in contextual draft request")
		}
		var excerpt string
		if err := json.Unmarshal([]byte(messages[1]["content"]), &excerpt); err != nil || excerpt != "Запись из [TERM]" {
			t.Fatalf("expected the selected line with an inline term marker, got %q", messages[1]["content"])
		}
		return `{"definition":"Журнал, запись из которого используется как фильтр заявки."}`, nil
	}
	got, err := g.SuggestDefinition(context.Background(), glossary.DefinitionRequest{Text: src, Abbreviation: abbr, Start: start, End: start + 3})
	if err != nil || calls != 2 || !got.Hypothesis || got.Text == "" || strings.Contains(got.Text, "подразделения") {
		t.Fatalf("expected a draft about the selected term, got %+v, %v", got, err)
	}
}

func TestDefinitionEvidenceSpansSelectedOccurrence(t *testing.T) {
	const source = "🔹 КП подписывает акт.\nКод: КП — номер\u00a0заявки."
	end := len([]rune("🔹 КП подписывает акт.\nКод: КП"))
	req := glossary.DefinitionRequest{Text: source, Abbreviation: "КП", Start: end - 2, End: end}
	for quote, want := range map[string]bool{"КП": false, "КП — номер заявки.": true, "КП подписывает акт.": false, "номер заявки.": false, "Код: КП — другой номер": false} {
		if got := mentionsOccurrence(quote, req); got != want {
			t.Errorf("quote %q: got %v, want %v", quote, got, want)
		}
	}
}
