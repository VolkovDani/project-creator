package contracts

import (
	"slices"
	"testing"
)

func TestClassificationAxesAndStatusVocabulary(t *testing.T) {
	c := ClassificationRules()
	if c.Version != "2" {
		t.Errorf("classification rules version = %q, want 2", c.Version)
	}
	if !slices.Contains(c.Representation, "test_case") || slices.Contains(c.Semantic, "test_case") {
		t.Errorf("test_case must be a representation only: %+v", c)
	}
	if !slices.Contains(c.Semantic, "functional_requirement") || !slices.Contains(c.Representation, "user_story") || slices.Contains(c.Semantic, "user_story") {
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
