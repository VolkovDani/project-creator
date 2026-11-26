package analysis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphBatchPlanStartsNewBatchForWholeRequirement(t *testing.T) {
	req := processgraph.Request{
		Anchor: processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{
			{ID: "r1", Version: 1, Text: "The queue starts.\n"},
			{ID: "r2", Version: 2, Text: " 🧾 The queue waits.\n\nThe queue closes.\n\t", Offset: 7},
		},
		OriginalSources: []processgraph.Source{
			{ID: "r1", Version: 1, Text: "The queue starts.\n"},
			{ID: "r2", Version: 2, Text: "Prefix  🧾 The queue waits.\n\nThe queue closes.\n\t"},
		},
	}
	budget := len(req.Sources[1].Text)
	plan, err := planProcessGraph(req, func(batch processgraph.Request) (bool, error) {
		size := 0
		for _, source := range batch.Sources {
			size += len(source.Text)
		}
		return size <= budget, nil
	})
	if err != nil || plan.Partial || len(plan.Batches) != len(req.Sources) {
		t.Fatalf("expected two complete batches: %+v, %v", plan, err)
	}
	for i, batch := range plan.Batches {
		if batch.Anchor != req.Anchor || !slices.Equal(batch.OriginalSources, req.OriginalSources) ||
			!slices.Equal(batch.Sources, req.Sources[i:i+1]) {
			t.Errorf("batch %d should preserve the metadata and the whole source: %+v", i, batch)
		}
	}
}

func TestProcessGraphBatchPlanUsesOneCallWhenManyShortRecordsFit(t *testing.T) {
	s := &chatSummary{managedNPU: true}
	req := processgraph.Request{Anchor: processgraph.Anchor{Name: "queue"}}
	for i := 0; i < 13; i++ {
		req.Sources = append(req.Sources, processgraph.Source{
			ID: strconv.Itoa(i), Version: 1, Text: "A queue starts.",
		})
	}
	plan, err := s.PlanProcessGraph(req)
	if err != nil || plan.Partial || len(plan.Batches) != 1 || len(plan.Batches[0].Sources) != len(req.Sources) {
		t.Fatalf("fitting originals should use one batch, got %d batches, partial=%t, error=%v", len(plan.Batches), plan.Partial, err)
	}
}

func TestProcessGraphBatchSplitAndSingleProviderAttempt(t *testing.T) {
	var calls int
	var failure error
	s := &chatSummary{
		managedNPU: true,
		verify:     context.Context.Err,
		complete: func(context.Context, []map[string]string, map[string]any) (string, error) {
			calls++
			return `{"r":[]}`, failure
		},
	}
	req := processgraph.Request{
		Anchor: processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{
			{ID: "r1", Version: 3, Text: "🧾 The queue begins.\n\nThe queue closes.", Offset: 7},
			{ID: "r2", Version: 4, Text: "The result is posted."},
		},
	}
	left, right, ok := s.SplitProcessGraphBatch(req)
	if !ok || left.Anchor != req.Anchor || right.Anchor != req.Anchor ||
		!slices.Equal(left.Sources, req.Sources[:1]) || !slices.Equal(right.Sources, req.Sources[1:]) ||
		!slices.Equal(left.OriginalSources, req.Sources) || !slices.Equal(right.OriginalSources, req.Sources) {
		t.Errorf("expected a split between whole requirements: %+v, %+v, %t", left, right, ok)
	}
	req.Sources = req.Sources[:1]
	if _, _, ok := s.SplitProcessGraphBatch(req); ok {
		t.Error("one requirement must be indivisible even with several passages")
	}

	for _, check := range []struct {
		providerErr, wantError error
	}{
		{nil, nil},
		{npuFailure("NPU_UNAVAILABLE"), processgraph.ErrBatchTooLarge},
		{npuFailure("NPU_WORKER_EXIT"), npuFailure("NPU_WORKER_EXIT")},
	} {
		calls, failure = 0, check.providerErr
		answer, err := s.GenerateProcessGraphBatch(t.Context(), req)
		if calls != 1 || !errors.Is(err, check.wantError) || err == nil && (answer.UsedSources != 1 || answer.Partial) {
			t.Fatalf("unexpected result for provider error %v: calls=%d answer=%+v error=%v", failure, calls, answer, err)
		}
	}
}

func TestProcessGraphBatchPlanSkipsOversizedRequirementAndKeepsLaterSource(t *testing.T) {
	s := &chatSummary{
		managedNPU: true,
		verify:     context.Context.Err,
		complete: func(context.Context, []map[string]string, map[string]any) (string, error) {
			t.Fatal("local budget checks should not call the provider")
			return "", nil
		},
	}
	const before = "The queue starts.\n"
	const after = "The queue closes."
	// JSON escapes markup characters in both the passage and the enclosing message.
	unfit := strings.Repeat("<", explanationQuoteMax) + "\n"
	req := processgraph.Request{
		Anchor: processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{
			{ID: "r1", Version: 1, Text: before + unfit + after},
			{ID: "r2", Version: 2, Text: "The queue records a result."},
		},
	}
	plan, err := s.PlanProcessGraph(req)
	if err != nil || !plan.Partial || len(plan.Batches) != 1 ||
		!slices.Equal(plan.Batches[0].Sources, req.Sources[1:]) ||
		!slices.Equal(plan.Batches[0].OriginalSources, req.Sources) {
		t.Fatalf("expected only the later source and partial coverage: %+v, %v", plan, err)
	}
	if _, err := s.GenerateProcessGraphBatch(t.Context(), req); !errors.Is(err, processgraph.ErrBatchTooLarge) {
		t.Fatalf("expected a local batch size rejection: %v", err)
	}
}

func TestProcessGraphBatchSplitsWhenTheProviderExhaustsOutputTokens(t *testing.T) {
	for _, tc := range []struct {
		name, provider string
		status         int
		body           string
		split          bool
	}{
		{"managed NPU length", "npu", 502, `{"error":{"code":"OUTPUT_TOKEN_LIMIT","message":"model provider failed"}}`, true},
		{"unrecognized NPU status", "npu", 503, `{"error":{"code":"OUTPUT_TOKEN_LIMIT","message":"model provider failed"}}`, false},
		{"invalid NPU output", "npu", 502, `{"error":{"code":"INVALID_OUTPUT","message":"model provider failed"}}`, false},
		{"compatible length", "compatible", 200, `{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"{\"r\":["}}]}`, true},
		{"compatible invalid JSON", "compatible", 200, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"r\":["}}]}`, false},
		{"Ollama length", "ollama", 200, `{"done":true,"done_reason":"length","message":{"role":"assistant","content":"{\"r\":["}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := processgraph.Request{
				Anchor: processgraph.Anchor{Name: "queue"},
				Sources: []processgraph.Source{
					{ID: "start", Version: 1, Text: "The queue begins."},
					{ID: "end", Version: 1, Text: "The queue closes."},
				},
			}
			const model = "test-chat:latest"
			digest := strings.Repeat("a", 64)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/tags":
					fmt.Fprintf(w, `{"models":[{"name":%q,"digest":%q}]}`, model, digest)
				case "/v1/chat/completions", "/api/chat":
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				default:
					t.Errorf("unexpected model route %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			var g processgraph.BatchGenerator
			var err error
			if tc.provider == "ollama" {
				g, err = NewSummaryOllamaWithLanguages(server.URL, model, digest, "en")
			} else {
				id := model
				if tc.provider == "npu" {
					id = "openvino-npu.test." + digest
				}
				g, err = NewSummaryOpenAICompatible(server.URL+"/v1", id, "", "en")
			}
			if err != nil {
				t.Fatal(err)
			}
			suggestion, err := g.GenerateProcessGraphBatch(t.Context(), req)
			if err == nil || errors.Is(err, processgraph.ErrBatchTooLarge) != tc.split ||
				suggestion.UsedSources != 0 || len(suggestion.Edges) != 0 {
				t.Fatalf("incomplete output must be discarded, split=%t: %+v, %v", tc.split, suggestion, err)
			}
		})
	}
}
