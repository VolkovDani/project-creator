package analysis

import (
	"context"
	"testing"
)

func TestServiceRejectsInventedReferences(t *testing.T) {
	pair := Pair{Left: Source{ID: "a", Version: 1, Text: "A"}, Right: Source{ID: "b", Version: 2, Text: "B"}}
	finding := Finding{LeftID: "invented", LeftVersion: 1, RightID: "b", RightVersion: 2, Verdict: "collision", Explanation: "Potential contradiction"}
	if validFinding(pair, finding) {
		t.Fatal("invented source accepted")
	}
	finding.LeftID = "a"
	if !validFinding(pair, finding) {
		t.Fatal("valid finding rejected")
	}
	for _, verdict := range []string{"probability", "", "certain"} {
		finding.Verdict = verdict
		if validFinding(pair, finding) {
			t.Fatal("unsupported verdict accepted")
		}
	}
}
func TestBoundedOutcome(t *testing.T) {
	if outcome([]Finding{{Verdict: "compatible"}}, false) != "no_collisions_found" {
		t.Fatal("compatible pair")
	}
	if outcome(nil, false) != "insufficient" {
		t.Fatal("empty retrieval claims success")
	}
	if outcome([]Finding{{Verdict: "compatible"}}, true) != "insufficient" {
		t.Fatal("incomplete retrieval claims success")
	}
	if outcome([]Finding{{Verdict: "collision"}}, true) != "possible_collision" {
		t.Fatal("possible collision missing")
	}
}

type generatorFunc func(context.Context, Pair) (Finding, error)

func (g generatorFunc) Profile() string                                         { return "test" }
func (g generatorFunc) Compare(ctx context.Context, pair Pair) (Finding, error) { return g(ctx, pair) }
func TestLateSuccessfulGeneratorIsTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pair := Pair{Left: Source{ID: "a", Version: 1, Text: "A"}, Right: Source{ID: "b", Version: 1, Text: "B"}}
	provider := generatorFunc(func(context.Context, Pair) (Finding, error) {
		cancel()
		return Finding{LeftID: "a", RightID: "b", LeftVersion: 1, RightVersion: 1, Verdict: "compatible", Explanation: "No contradiction in this pair."}, nil
	})
	_, code := checkedCompare(ctx, provider, pair)
	if code != "TIMEOUT" {
		t.Fatalf("late successful completion classified as %q", code)
	}
}
