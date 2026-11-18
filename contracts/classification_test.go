package contracts

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestClassificationAxesAndStatusVocabulary(t *testing.T) {
	var c struct {
		Version                          string
		Semantic, Representation, States []string
	}
	raw, err := os.ReadFile("classification.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c.Version == "" || !slices.Contains(c.Semantic, "functional_requirement") || !slices.Contains(c.Representation, "user_story") || slices.Contains(c.Semantic, "user_story") {
		t.Fatalf("independent axes missing: %+v", c)
	}
	for _, axis := range [][]string{c.Semantic, c.Representation} {
		if !slices.Contains(axis, "unknown") || !slices.Contains(axis, "other") {
			t.Fatalf("missing fallback: %v", axis)
		}
	}
	want := []string{"unclassified", "queued", "classifying", "suggested", "needs_review", "confirmed", "failed", "stale"}
	if !slices.Equal(c.States, want) {
		t.Fatalf("classification states: %v", c.States)
	}
}
