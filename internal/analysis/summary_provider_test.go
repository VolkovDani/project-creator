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
			json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"Пользователь обязан сохранить \"да\"\nпри любой роли 👩‍🔬.","unsummarizable":false}`}})
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
	if !strings.Contains(prompt, "Remove repeated wording") || !strings.Contains(prompt, "Cyrillic text often uses two bytes") || !strings.Contains(prompt, "before refusing") {
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

func TestSummaryOllamaInvalidOutputRetryAndImpossible(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents []string
		wantErr  error
		calls    int
	}{
		{"retry-overlong", []string{`{"summary":"` + strings.Repeat("x", 351) + `","unsummarizable":false}`, `{"summary":"short","unsummarizable":false}`}, nil, 2},
		{"impossible", []string{`{"summary":"","unsummarizable":true}`}, ErrSummaryImpossible, 1},
		{"retry-invalid", []string{`{"summary":"short","unsummarizable":"yes"}`, `{"summary":"short","unsummarizable":false,"extra":1}`}, ErrInvalidSummary, 2},
		{"too-long", []string{`{"summary":"` + strings.Repeat("x", 351) + `","unsummarizable":false}`, `{"summary":"` + strings.Repeat("y", 351) + `","unsummarizable":false}`}, ErrInvalidSummary, 2},
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
			} else if err != nil || got != "short" {
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
		content := `{"summary":"` + strings.Repeat("x", 40000) + `","unsummarizable":false}`
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
				json.NewEncoder(w).Encode(map[string]any{"done": scenario != "incomplete", "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"short","unsummarizable":false}`}})
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
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"summary":"short","unsummarizable":false}`}})
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
	if ValidateSummary(strings.Repeat("x", contracts.SummaryMaxBytes()+1)) != ErrInvalidSummary {
		t.Fatal("over-limit summary accepted")
	}
}

func TestSummarySchemaDraftsBeforeRefusalFlag(t *testing.T) {
	raw, err := json.Marshal(summarySchema())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	summaryAt, flagAt := strings.Index(text, "summary"), strings.Index(text, "unsummarizable")
	if summaryAt < 0 || flagAt < 0 || summaryAt > flagAt {
		t.Fatalf("schema must draft summary before unsummarizable flag: %s", text)
	}
	if _, err = decodeSummary(`{"summary":"","unsummarizable":true}`); err != ErrSummaryImpossible {
		t.Fatalf("refusal err=%v", err)
	}
	if _, err = decodeSummary(`{"summary":"faithful","unsummarizable":false}`); err != nil {
		t.Fatalf("valid summary err=%v", err)
	}
	for _, raw := range []string{`{"summary":"contradiction","unsummarizable":true}`, `{"summary":"","unsummarizable":false}`} {
		if _, err = decodeSummary(raw); err != ErrInvalidSummary {
			t.Errorf("accepted contradictory output %s: %v", raw, err)
		}
	}
}

func TestSummaryOllamaRespectsCancellation(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			summaryTags(w, summaryDigest)
			return
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-finish:
		}
	}))
	defer server.Close()
	g, err := NewSummaryOllama(server.URL, summaryModel, summaryDigest)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, callErr := g.Summarize(ctx, summarySource("A must be preserved.")); done <- callErr }()
	<-started
	cancel()
	if err = <-done; err == nil {
		t.Fatal("cancelled call succeeded")
	}
	close(finish)
}
