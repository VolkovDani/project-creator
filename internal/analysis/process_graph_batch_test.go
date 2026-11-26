package analysis

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphBatchPlanKeepsWholePassagesAndSourceOrder(t *testing.T) {
	s := &chatSummary{managedNPU: true}
	short := processgraph.Request{
		Anchor: processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{
			{ID: "r1", Version: 1, Text: "The queue starts.\n"},
			{ID: "r2", Version: 2, Text: "The queue ends."},
		},
	}
	plan, err := s.PlanProcessGraph(short)
	if err != nil || plan.Partial || len(plan.Batches) != 1 || len(plan.Batches[0].Sources) != 2 {
		t.Fatalf("small request should use one batch: %+v, %v", plan, err)
	}

	start := "🧾 The queue begins.\n"
	passage := strings.Repeat("record ", 70) + "waits.\n"
	end := "The queue closes."
	short.Sources[0].Text = start + strings.Repeat(passage, 25) + end
	plan, err = s.PlanProcessGraph(short)
	if err != nil || plan.Partial || len(plan.Batches) < 2 {
		t.Fatalf("long request should use multiple complete batches: %+v, %v", plan, err)
	}
	var rebuilt strings.Builder
	var sawLaterSource bool
	for _, batch := range plan.Batches {
		if batch.Anchor != short.Anchor {
			t.Fatalf("batch lost its anchor: %+v", batch.Anchor)
		}
		for _, source := range batch.Sources {
			if source.ID == "r2" {
				sawLaterSource = true
				if source != short.Sources[1] {
					t.Fatalf("later source changed: %+v", source)
				}
				continue
			}
			if sawLaterSource || source.Version != 1 || source.Offset != utf8.RuneCountInString(rebuilt.String()) {
				t.Fatalf("source fragment is out of order or has the wrong original offset: %+v", source)
			}
			if source.Text == "" || !strings.HasSuffix(source.Text, "\n") && !strings.HasSuffix(source.Text, end) {
				t.Fatalf("batch cuts a source passage: %q", source.Text)
			}
			rebuilt.WriteString(source.Text)
		}
	}
	if rebuilt.String() != short.Sources[0].Text || !sawLaterSource {
		t.Fatal("planning lost or repeated later source passages")
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
	const first = "🧾 The queue begins.\n"
	const second = "The queue closes."
	req := processgraph.Request{
		Anchor:  processgraph.Anchor{Name: "queue"},
		Sources: []processgraph.Source{{ID: "r1", Version: 3, Text: first + second, Offset: 7}},
	}
	left, right, ok := s.SplitProcessGraphBatch(req)
	if !ok || len(left.Sources) != 1 || len(right.Sources) != 1 || left.Anchor != req.Anchor || right.Anchor != req.Anchor ||
		left.Sources[0] != (processgraph.Source{ID: "r1", Version: 3, Text: first, Offset: 7}) ||
		right.Sources[0] != (processgraph.Source{ID: "r1", Version: 3, Text: second, Offset: 7 + utf8.RuneCountInString(first)}) {
		t.Fatalf("unexpected passage split: %+v, %+v, %t", left, right, ok)
	}
	if _, _, ok := s.SplitProcessGraphBatch(right); ok {
		t.Fatal("one complete passage is indivisible")
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

func TestProcessGraphBatchPlanReportsUnfitPassageAndKeepsLaterSource(t *testing.T) {
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
	if err != nil || !plan.Partial || len(plan.Batches) != 2 || len(plan.Batches[0].Sources) != 1 ||
		len(plan.Batches[1].Sources) != 2 || plan.Batches[0].Sources[0].Text != before ||
		plan.Batches[1].Sources[0].Text != after || plan.Batches[1].Sources[0].Offset != len(before+unfit) ||
		plan.Batches[1].Sources[1] != req.Sources[1] {
		t.Fatalf("expected a bounded gap followed by later source coverage: %+v, %v", plan, err)
	}
	if _, err := s.GenerateProcessGraphBatch(t.Context(), req); !errors.Is(err, processgraph.ErrBatchTooLarge) {
		t.Fatalf("expected a local batch size rejection: %v", err)
	}
}
