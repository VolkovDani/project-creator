package summary

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

func TestLongPreviewIsSavedAndSkipsOnlyLaya(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec(`INSERT INTO requirements VALUES('second','a','p','Second original',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("Полезная выжимка с полными существенными условиями. ", 12)
	g := &fakeGenerator{text: long}
	s := New(db, g)
	defer s.Close()
	if err := s.Start([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	left := waitState(t, s, "a", "p", "r", "ready")
	right := waitState(t, s, "a", "p", "second", "ready")
	for _, v := range []State{left, right} {
		if v.Text != long || v.ByteLength != len(long) || v.LayaEligible {
			t.Fatalf("preview not preserved or admitted to Laya: status=%s bytes=%d eligible=%v", v.Status, v.ByteLength, v.LayaEligible)
		}
	}
	classifier := &captureClassifier{}
	final := &optionalFinalGenerator{}
	analyzer := analysis.New(db, nil, final, "", s.Classifier(classifier))
	defer analyzer.Close()
	r, err := analyzer.StartWithOptions(context.Background(), "a", "p", analysis.Options{Mode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		v, e := analyzer.Get(context.Background(), "a", "p", r.ID)
		if e != nil {
			t.Fatal(e)
		}
		if v.Status != "running" {
			if v.CompletedPairs != 1 || v.FailedPairs != 0 || v.LayaSkippedPairs != 1 {
				t.Fatalf("unexpected fallback result: completed=%d failed=%d skipped=%d", v.CompletedPairs, v.FailedPairs, v.LayaSkippedPairs)
			}
			if classifier.called.Load() != 0 || final.calls.Load() != 1 || final.bad.Load() {
				t.Fatalf("invalid model routing: laya=%d llm=%d bad=%v", classifier.called.Load(), final.calls.Load(), final.bad.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fallback analysis did not finish")
}

type optionalFinalGenerator struct {
	calls atomic.Int32
	bad   atomic.Bool
}

func (g *optionalFinalGenerator) Profile() string { return "optional-final-v1" }
func (g *optionalFinalGenerator) Compare(_ context.Context, p analysis.Pair) (analysis.Finding, error) {
	g.calls.Add(1)
	g.bad.Store(p.Preclassification != nil || strings.Contains(p.Left.Text, "Полезная выжимка") || strings.Contains(p.Right.Text, "Полезная выжимка"))
	return analysis.Finding{LeftID: p.Left.ID, RightID: p.Right.ID, LeftVersion: p.Left.Version, RightVersion: p.Right.Version, Verdict: "compatible", Explanation: "Full originals compared without Laya"}, nil
}
