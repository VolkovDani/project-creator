package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/contracts"
)

const summaryModel = "local-summary-test"
const summaryDigest = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

func summaryTags(w http.ResponseWriter, digest string) {
	fmt.Fprintf(w, `{"models":[{"name":%q,"digest":%q}]}`, summaryModel, "sha256:"+digest)
}

func summarySource(text string) Source { return Source{ID: "req-1", Version: 3, Text: text} }

func TestSummaryOllamaValidatesAndPreservesProvenance(t *testing.T) {
	var request map[string]any
	tags := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			tags++
			summaryTags(w, summaryDigest)
		case "/api/chat":
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"Пользователь обязан сохранить \"да\"\nпри любой роли 👩‍🔬."}`}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("обязательство ", 600) // >2,000 runes, within the requirement contract.
	got, err := g.Summarize(context.Background(), summarySource(text))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Пользователь обязан сохранить \"да\"\nпри любой роли 👩‍🔬." || tags != 2 {
		t.Fatalf("summary=%q tags=%d", got, tags)
	}
	if request["model"] != summaryModel || request["stream"] != false {
		t.Fatalf("request=%#v", request)
	}
	messages := request["messages"].([]any)
	if !strings.Contains(messages[0].(map[string]any)["content"].(string), fmt.Sprintf("%d UTF-8 bytes", contracts.SummaryMaxBytes())) {
		t.Fatal("prompt omits numeric summary byte budget")
	}
	prompt := messages[0].(map[string]any)["content"].(string)
	if !strings.Contains(prompt, "Remove repeated wording") || !strings.Contains(prompt, "soft target") || !strings.Contains(prompt, "longer faithful preview") {
		t.Fatalf("prompt omits concise byte-aware strategy: %s", prompt)
	}
	if !strings.Contains(prompt, "Preserve exclusivity words such as only and только") || !strings.Contains(prompt, "Preserve negative obligations as explicit negative verbs") || !strings.Contains(prompt, "topic fragment") {
		t.Fatalf("prompt omits exclusivity/negation safeguards: %s", prompt)
	}
	var p map[string]any
	if err = json.Unmarshal([]byte(g.Provenance()), &p); err != nil {
		t.Fatal(err)
	}
	if p["digest"] != summaryDigest || p["model"] != summaryModel || g.Profile() == "" || p["responseSchema"] == nil {
		t.Fatalf("provenance=%#v", p)
	}
}

func TestSummaryOllamaInvalidOutputRetryAndLongPreview(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents []string
		wantErr  error
		calls    int
	}{
		{"source-language-corrected", []string{`{"summary":"Сохранить требование."}`, `{"summary":"Save the requirement."}`}, nil, 2},
		{"source-language-exhausted", []string{`{"summary":"Сохранить требование."}`}, ErrInvalidSummary, 2},
		{"long-valid", []string{`{"summary":"` + strings.Repeat("x", 351) + `"}`}, nil, 1},
		{"foreign-letters-invalid", []string{`{"summary":"必须保存要求"}`}, ErrInvalidSummary, 2},
		{"foreign-letters-corrected", []string{`{"summary":"必须保存要求"}`, `{"summary":"Save the requirement."}`}, nil, 2},
		{"empty-invalid", []string{`{"summary":""}`}, ErrInvalidSummary, 2},
		{"retry-invalid", []string{`{"summary":"short","unsummarizable":"yes"}`, `{"summary":"short","extra":1}`}, ErrInvalidSummary, 2},
		{"invalid-shape", []string{`{"summary":null}`}, ErrInvalidSummary, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var secondRequest map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					summaryTags(w, summaryDigest)
					return
				}
				calls++
				var received map[string]any
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				if calls == 2 {
					secondRequest = received
				}
				i := calls - 1
				if i >= len(tc.contents) {
					i = len(tc.contents) - 1
				}
				json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": tc.contents[i]}})
			}))
			defer server.Close()
			g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
			if err != nil {
				t.Fatal(err)
			}
			got, err := g.Summarize(context.Background(), summarySource("Original requirement"))
			if calls != tc.calls {
				t.Fatalf("calls=%d want=%d", calls, tc.calls)
			}
			if tc.calls == 2 {
				messages := secondRequest["messages"].([]any)
				if len(messages) != 4 || !strings.Contains(messages[3].(map[string]any)["content"].(string), fmt.Sprintf("%d UTF-8 bytes", contracts.SummaryMaxBytes())) {
					t.Fatalf("retry lacks bounded corrective feedback: %#v", secondRequest)
				}
				if tc.name == "retry-overlong" && !strings.Contains(messages[3].(map[string]any)["content"].(string), "351 UTF-8 bytes") {
					t.Fatalf("feedback omits actual summary byte length: %#v", messages[3])
				}
			}
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("err=%v want=%v", err, tc.wantErr)
				}
			} else if err != nil || (tc.name == "long-valid" && got != strings.Repeat("x", 351)) {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

func TestSummaryOllamaDoesNotRetryWhenCorrectionExceedsContextBudget(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		calls++
		content := `{"summary":"` + strings.Repeat("x", 40000) + `","extra":true}`
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}})
	}))
	defer server.Close()
	g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Summarize(context.Background(), summarySource("A requirement longer than 350 bytes."))
	if err != ErrInvalidSummary || calls != 1 {
		t.Fatalf("err=%v inference calls=%d", err, calls)
	}
}

func TestSummaryOllamaRejectsInvalidSourceIncompleteResponseAndDigestDrift(t *testing.T) {
	for _, scenario := range []string{"too-many-runes", "invalid-utf8", "incomplete", "digest-drift"} {
		t.Run(scenario, func(t *testing.T) {
			tags := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					tags++
					d := summaryDigest
					if scenario == "digest-drift" && tags > 1 {
						d = strings.Repeat("e", 64)
					}
					summaryTags(w, d)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"done": scenario != "incomplete", "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"short"}`}})
			}))
			defer server.Close()
			g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
			if err != nil {
				t.Fatal(err)
			}
			src := summarySource("valid")
			switch scenario {
			case "too-many-runes":
				src.Text = strings.Repeat("я", 10001)
			case "invalid-utf8":
				src.Text = string([]byte{0xff})
			}
			_, err = g.Summarize(context.Background(), src)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if scenario == "digest-drift" && tags != 2 {
				t.Fatalf("verification count=%d", tags)
			}
		})
	}
}

func TestSummaryOllamaRejectsEscapedInputBeforeRuntimeButAllowsLongText(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"Кратко"}`}})
	}))
	defer server.Close()
	g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Summarize(context.Background(), summarySource(strings.Repeat("\x01", 10000)))
	if err != ErrSummaryInputTooLong || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	_, err = g.Summarize(context.Background(), summarySource(strings.Repeat("обычное требование ", 400)))
	if err != nil {
		t.Fatalf("long normal requirement rejected: %v", err)
	}
}

func TestValidateSummaryUsesSharedContract(t *testing.T) {
	if ValidateSummary(strings.Repeat("x", contracts.SummaryMaxBytes())) != nil {
		t.Fatal("contract maximum rejected")
	}
	if ValidateSummary(strings.Repeat("x", contracts.SummaryMaxBytes()+1)) != nil {
		t.Fatal("long preview was rejected")
	}
}

func TestSummarySchemaAllowsFaithfulLongPreviewWithoutRefusalFlag(t *testing.T) {
	if got, e := decodeSummary(`{"summary":"A faithful preview"}`); e != nil || got != "A faithful preview" {
		t.Fatalf("preview=%q err=%v", got, e)
	}
	if _, e := decodeSummary(`{"summary":"x","unsummarizable":false}`); e != ErrInvalidSummary {
		t.Fatal("obsolete response flag was accepted")
	}
}

func TestSummaryOllamaPreservesLongPreviewWithoutRetry(t *testing.T) {
	calls := 0
	want := strings.Repeat("Существенное условие. ", 30)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		calls++
		raw, _ := json.Marshal(map[string]string{"summary": want})
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(raw)}})
	}))
	defer server.Close()
	g, e := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if e != nil {
		t.Fatal(e)
	}
	got, e := g.Summarize(context.Background(), summarySource("Полный оригинал"))
	if e != nil || got != want || calls != 1 {
		t.Fatalf("long preview lost: bytes=%d calls=%d err=%v", len(got), calls, e)
	}
}

func TestSummaryRetriesWhenNumbersOrIdentifiersAreOmitted(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		calls++
		text := "Retain the report."
		if calls == 2 {
			text = "Retain REQ-0042 for 90 days."
		}
		raw, _ := json.Marshal(map[string]string{"summary": text})
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(raw)}})
	}))
	defer server.Close()
	g, e := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if e != nil {
		t.Fatal(e)
	}
	got, e := g.Summarize(context.Background(), summarySource("Retain REQ-0042 for 90 days."))
	if e != nil || calls != 2 || got != "Retain REQ-0042 for 90 days." {
		t.Fatalf("source facts lost: text=%q calls=%d err=%v", got, calls, e)
	}
}

func TestSummaryLanguageConfigurationChangesProfileAndValidation(t *testing.T) {
	defaultGenerator, err := NewSummaryOllama("http://127.0.0.1:11434", summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	german, err := NewSummaryOllamaWithLanguages("http://127.0.0.1:11434", summaryModel, summaryDigest, "en,de")
	if err != nil {
		t.Fatal(err)
	}
	if defaultGenerator.Profile() == german.Profile() {
		t.Fatal("language change reused the old profile")
	}
	if defaultGenerator.ValidateSummary("Prüfung 42 €") == nil || german.ValidateSummary("Prüfung 42 €") != nil {
		t.Fatal("configured alphabet not enforced")
	}
	if !strings.Contains(german.prompt, "German") || strings.Contains(german.prompt, "in the same language as the source") {
		t.Fatal("prompt contradicts output language policy")
	}
	var provenance map[string]any
	if json.Unmarshal([]byte(german.Provenance()), &provenance) != nil || !strings.Contains(german.Provenance(), "allowedLanguages") {
		t.Fatal("missing language provenance")
	}
}

func TestSummaryOllamaPreservesSourceTechnicalIdentifiersOnly(t *testing.T) {
	tests := []struct {
		name, source, output string
		wantErr              bool
		wantCalls            int
	}{
		{
			name:      "preserve source technical names including lowercase Excel",
			source:    "Требование: выгрузить в excel через API в PDF.",
			output:    "Нужно выгрузить Excel через API в PDF.",
			wantCalls: 1,
		},
		{
			name:      "reject English prose around a permitted identifier",
			source:    "Требование: сохранить Excel и API.",
			output:    "Save the Excel API.",
			wantErr:   true,
			wantCalls: 2,
		},
		{
			name:      "reject a novel foreign name",
			source:    "Требование: сохранить Excel и API.",
			output:    "Нужно сохранить Excel и Acme.",
			wantErr:   true,
			wantCalls: 2,
		},
		{
			name:      "do not whitelist names from a source foreign prose run",
			source:    "Описание: export all records, сохранить Excel.",
			output:    "Нужно сохранить Excel.",
			wantErr:   true,
			wantCalls: 2,
		},
		{
			name:      "preserve source PascalCase technical names without a catalog",
			source:    "Требование: сохранить LibreOffice через ZetaTool.",
			output:    "Нужно сохранить LibreOffice через ZetaTool.",
			wantCalls: 1,
		},
		{
			name:      "reject Unicode lookalike letters",
			source:    "Требование: сохранить Excel.",
			output:    "Нужно сохранить Excеl.", // The fourth character is Cyrillic small ie.
			wantErr:   true,
			wantCalls: 2,
		},
		{
			name:      "reject output made only of identifiers",
			source:    "Требование: сохранить Excel и API.",
			output:    "Excel API",
			wantErr:   true,
			wantCalls: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					summaryTags(w, summaryDigest)
					return
				}
				calls++
				content, err := json.Marshal(map[string]string{"summary": tc.output})
				if err != nil {
					t.Error(err)
				}
				json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(content)}})
			}))
			defer server.Close()

			g, err := NewSummaryOllamaWithLanguages(server.URL, summaryModel, summaryDigest, "ru")
			if err != nil {
				t.Fatal(err)
			}
			got, err := g.Summarize(context.Background(), summarySource(tc.source))
			if (err != nil) != tc.wantErr {
				t.Fatalf("summary=%q err=%v wantErr=%v", got, err, tc.wantErr)
			}
			if calls != tc.wantCalls {
				t.Fatalf("generation calls=%d want=%d", calls, tc.wantCalls)
			}
			if !tc.wantErr && got != tc.output {
				t.Fatalf("summary=%q want=%q", got, tc.output)
			}
			if !tc.wantErr && g.ValidateSummary(got) == nil {
				t.Fatal("source-less validation accepted technical identifiers")
			}
			if !strings.Contains(g.prompt, "technical names from the source") || !strings.Contains(g.prompt, "all other wording") {
				t.Fatalf("prompt omits technical-name policy: %s", g.prompt)
			}
		})
	}
}

func TestSummaryOllamaRejectsNULEvenWithSourceTechnicalNames(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		calls++
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"Сохранить Excel\u0000"}`}})
	}))
	defer server.Close()
	g, err := NewSummaryOllamaWithLanguages(server.URL, summaryModel, summaryDigest, "ru")
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Summarize(context.Background(), summarySource("Требование: сохранить Excel."))
	if err != ErrInvalidSummary || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
