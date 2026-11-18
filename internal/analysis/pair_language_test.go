package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDecodeFindingAlphabet(t *testing.T) {
	for _, tc := range []struct {
		name, explanation string
		valid             bool
	}{
		{"Russian and ASCII terms", "Ёмкость API — 42%, «ещё» ٢ × 3 ✅", true},
		{"longer than a summary", strings.Repeat("API ", 499) + "Да", true},
		{"over pair length limit", strings.Repeat("Я", 2001), false},
		{"Han", "两项需求矛盾。", false},
		{"Japanese kana", "ルールは異なる。", false},
		{"isolated insertion", "Правила 界 различаются.", false},
		{"quoted source text", "В документе написано «日本語».", false},
		{"accented Latin", "Термин café используется в API.", false},
		{"non-Russian Cyrillic", "Термин ї используется в API.", false},
		{"combining mark", "Термин APÍ используется.", false},
		{"control character", "API\x00 используется.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := decodeFinding(pairLanguageFinding(t, pairFixture(), "collision", tc.explanation), pairFixture())
			if tc.valid {
				if err != nil || result.Explanation != tc.explanation {
					t.Fatalf("valid explanation changed or rejected: %+v, %v", result, err)
				}
			} else if err == nil || result != (Finding{}) {
				t.Fatalf("invalid explanation did not produce an empty finding and error: %+v, %v", result, err)
			}
		})
	}
}

func pairLanguageFinding(t *testing.T, pair Pair, verdict, explanation string) string {
	t.Helper()
	data, err := json.Marshal(Finding{
		LeftID: pair.Left.ID, RightID: pair.Right.ID,
		LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version,
		Verdict: verdict, Explanation: explanation,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type pairLanguageTransport func(*http.Request) (*http.Response, error)

func (f pairLanguageTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestPairAdaptersAlphabetRetry(t *testing.T) {
	const valid = "Условия API совпадают, но способы оплаты различаются."
	const invalid = "Запрещённая вставка «漢字»."
	for _, adapter := range []string{"Ollama", "OpenVINO"} {
		t.Run(adapter, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				explanations []string
				verdict      string
				cancel       bool
				deadline     bool
				success      bool
			}{
				{name: "valid first answer", explanations: []string{valid}, success: true},
				{name: "retry succeeds", explanations: []string{invalid, valid}, success: true},
				{name: "second violation fails", explanations: []string{invalid, invalid}},
				{name: "unknown verdict is terminal", explanations: []string{invalid}, verdict: "unknown"},
				{name: "cancellation prevents retry", explanations: []string{invalid}, cancel: true},
				{name: "retry uses remaining budget", explanations: []string{invalid, ""}, deadline: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					pair := pairFixture()
					pair.Left.Text = "A «日本語»"
					pair.Right.Text = "B «café»"
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					var originalDeadline time.Time
					if tc.deadline {
						originalDeadline = time.Now().Add(500 * time.Millisecond)
						var stop context.CancelFunc
						ctx, stop = context.WithDeadline(ctx, originalDeadline)
						defer stop()
					}
					generations := 0
					var firstPrompt string
					transport := pairLanguageTransport(func(r *http.Request) (*http.Response, error) {
						switch r.URL.Path {
						case "/api/tags":
							return pairLanguageResponse(fmt.Sprintf(`{"models":[{"name":"qwen2.5:7b","digest":"sha256:%s"}]}`, pinnedDigest)), nil
						case "/health":
							return pairLanguageResponse(fmt.Sprintf(`{"profile":"%s","device":"NPU"}`, pinnedDigest)), nil
						case "/api/chat", "/compare":
						default:
							t.Fatalf("unexpected request %s", r.URL.Path)
						}
						generations++
						if generations > len(tc.explanations) {
							t.Fatalf("unexpected generation %d", generations)
						}
						if tc.deadline {
							actual, ok := r.Context().Deadline()
							if !ok || !actual.Equal(originalDeadline) {
								t.Fatalf("generation %d deadline = %v, want %v", generations, actual, originalDeadline)
							}
							if generations == 2 {
								<-r.Context().Done()
								return nil, r.Context().Err()
							}
						}
						prompt, supplied, raw := readPairLanguageRequest(t, r)
						expected := pair
						if adapter == "OpenVINO" {
							expected.Left.Version, expected.Right.Version = 1, 1
						}
						if !reflect.DeepEqual(supplied, expected) {
							t.Errorf("generation %d received %+v, want %+v", generations, supplied, expected)
						}
						if generations == 1 {
							firstPrompt = prompt
						} else {
							if prompt == firstPrompt || !strings.Contains(prompt, firstPrompt) {
								t.Error("retry did not strengthen the original instructions")
							}
							if strings.Contains(raw, "漢字") || strings.Contains(raw, "Запрещённая вставка") {
								t.Error("retry shared rejected content")
							}
						}
						verdict := tc.verdict
						if verdict == "" {
							verdict = "collision"
						}
						content := pairLanguageFinding(t, expected, verdict, tc.explanations[generations-1])
						if tc.cancel {
							cancel()
						}
						if adapter == "OpenVINO" {
							return pairLanguageResponse(fmt.Sprintf(`{"profile":"%s","device":"NPU","finding":%s}`, pinnedDigest, content)), nil
						}
						return pairLanguageResponse(fmt.Sprintf(`{"done":true,"done_reason":"stop","message":{"role":"assistant","content":%s}}`, pairLanguageJSONString(t, content))), nil
					})
					generator := pairLanguageGenerator(t, adapter, transport)
					result, err := generator.Compare(ctx, pair)
					if generations != len(tc.explanations) {
						t.Errorf("got %d generations, want %d", generations, len(tc.explanations))
					}
					if tc.success {
						if err != nil || result.Explanation != valid || !validFinding(pair, result) {
							t.Fatalf("expected successful finding for the original pair: %+v, %v", result, err)
						}
					} else if err == nil || result != (Finding{}) || strings.Contains(err.Error(), "漢字") {
						t.Fatalf("expected sanitized error and empty finding: %+v, %v", result, err)
					}
				})
			}
		})
	}
}

func readPairLanguageRequest(t *testing.T, r *http.Request) (string, Pair, string) {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		System   string `json:"system"`
		Pair     Pair   `json:"pair"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	if r.URL.Path == "/api/chat" {
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Fatalf("unexpected messages: %+v", request.Messages)
		}
		request.System = request.Messages[0].Content
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &request.Pair); err != nil {
			t.Fatal(err)
		}
	}
	return request.System, request.Pair, string(data)
}

func pairLanguageGenerator(t *testing.T, adapter string, transport http.RoundTripper) Generator {
	t.Helper()
	if adapter == "Ollama" {
		generator, err := NewOllama("http://localhost:11434", "qwen2.5:7b", pinnedDigest)
		if err != nil {
			t.Fatal(err)
		}
		generator.client.Transport = transport
		return generator
	}
	generator, err := NewOpenVINO("/run/npu/runtime.sock", pinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	generator.client.Transport = transport
	return generator
}

func pairLanguageResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func pairLanguageJSONString(t *testing.T, content string) string {
	t.Helper()
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
