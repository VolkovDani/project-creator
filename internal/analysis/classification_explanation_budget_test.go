package analysis

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/explanation"
)

func TestManagedExplanationUsesTheProfileTokenBudget(t *testing.T) {
	const model = "openvino-npu.test." + pinnedDigest
	const statement = `{"assessment":{"verdict":"fits","reason":"The requirement is a statement.","criteria":{"declarative_assertion_or_directive":{"status":"present","evidence":1,"note":"The statement is explicit."}}},"questions":[],"wording":"","limitations":"One requirement was reviewed."}`
	source := explanation.Source{
		Text:       strings.Repeat("Export annual receipts. ", 350),
		Suggestion: &classification.Entry{Semantic: "functional_requirement", Representation: "statement"},
	}
	target := explanation.Target{Semantic: "functional_requirement", Representation: "statement", Locale: "en"}
	for _, tc := range []struct {
		name, measuredModel                 string
		inputTokens, formLimit, completions int
		status                              int
		approximate                         bool
		want                                error
	}{
		{name: "profile accepts a request larger than 6500 bytes", measuredModel: model, inputTokens: 2049, formLimit: 4096, completions: 2},
		{name: "reject second axis before sending either generation", measuredModel: model, inputTokens: 2049, formLimit: 2048, want: explanation.ErrInputTooLong},
		{name: "older manager delegates enforcement to the provider", status: http.StatusNotFound, completions: 2},
		{name: "explicitly unsupported count delegates enforcement to the provider", status: http.StatusNotImplemented, completions: 2},
		{name: "profile transport rejects the complete request", status: http.StatusRequestEntityTooLarge, want: explanation.ErrInputTooLong},
		{name: "estimated limit defers to runtime enforcement", measuredModel: model, inputTokens: 4100, formLimit: 4096, approximate: true, completions: 2},
		{name: "wrong model invalidates the measurement", measuredModel: "another-profile", inputTokens: 1024, formLimit: 4096, want: errInvalidTestBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var counts, completions int
			probed := map[string]string{}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wire, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var body struct {
					Model      string
					Messages   []map[string]string
					Completion *int `json:"max_completion_tokens"`
				}
				if err := json.Unmarshal(wire, &body); err != nil || len(body.Messages) != 2 || body.Model != model {
					t.Errorf("unexpected profile request: %v", err)
					return
				}
				if body.Completion != nil {
					t.Error("explicit application output cap overrides the active profile")
				}
				axis := "content"
				if strings.Contains(body.Messages[0]["content"], `"axis":"form"`) {
					axis = "form"
				}
				switch r.URL.Path {
				case "/v1/chat/token-count":
					counts++
					probed[axis] = string(wire)
					if counts == 1 && len(wire) <= 6500 {
						t.Error("fixture must exceed the old byte budget")
					}
					if tc.status != 0 {
						w.WriteHeader(tc.status)
						return
					}
					limit := 4096
					if axis == "form" {
						limit = tc.formLimit
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"model": tc.measuredModel, "input_tokens": tc.inputTokens, "max_input_tokens": limit, "max_completion_tokens": 2048, "exact": !tc.approximate})
				case "/v1/chat/completions":
					completions++
					if probed[axis] != string(wire) {
						t.Error("generation request differs from the measured body")
					}
					content := statement
					if axis == "content" {
						content = explanationAxisResponse(t, explanationSample, axis)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
						"finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": content},
					}}})
				default:
					t.Errorf("unexpected provider endpoint %q", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer provider.Close()
			g, err := NewOpenAICompatible(provider.URL+"/v1", model, "")
			if err != nil {
				t.Fatal(err)
			}
			got, err := g.ExplainClassification(t.Context(), source, target)
			switch tc.want {
			case errInvalidTestBudget:
				if err == nil || errors.Is(err, explanation.ErrInputTooLong) {
					t.Fatalf("expected invalid measurement, got %v", err)
				}
			default:
				if !errors.Is(err, tc.want) {
					t.Fatalf("returned %v, want %v", err, tc.want)
				}
			}
			if counts < 1 || completions != tc.completions {
				t.Fatalf("got %d measurements and %d completions, expected %d completions", counts, completions, tc.completions)
			}
			if tc.completions == 2 && (got.Meaning.Criteria[0].Evidence != "Export annual receipts." || got.Form.Criteria[0].Evidence != "Export annual receipts.") {
				t.Fatal("the measured request must preserve the full source and verbatim references")
			}
		})
	}
}

var errInvalidTestBudget = errors.New("invalid measured budget")
