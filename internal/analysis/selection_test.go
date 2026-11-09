package analysis

import (
	"reflect"
	"testing"
)

func TestSelectionSemanticThresholdIsRequired(t *testing.T) {
	pair := Pair{Left: Source{Text: "Invoice retention lasts thirty days."}, Right: Source{Text: "Invoice retention ends after sixty days."}}
	for _, tc := range []struct {
		name      string
		mode      string
		threshold float64
		score     *float64
		exact     bool
		want      []string
	}{
		{name: "shared words below threshold", mode: "related", threshold: .9, score: selectionScore(.8), want: []string{}},
		{name: "shared words without score", mode: "related", threshold: .9, want: []string{}},
		{name: "at threshold", mode: "related", threshold: .8, score: selectionScore(.8), want: []string{"semantic_similarity"}},
		{name: "above threshold", mode: "related", threshold: .7, score: selectionScore(.8), want: []string{"semantic_similarity"}},
		{name: "exact duplicate exception", mode: "related", threshold: .9, score: selectionScore(.8), exact: true, want: []string{"exact_text"}},
		{name: "all pairs", mode: "all", threshold: .9, score: selectionScore(.8), want: []string{"all_pairs"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := pair
			if tc.exact {
				candidate.Right.Text = candidate.Left.Text
			}
			if got := selection(candidate, tc.mode, tc.threshold, tc.score); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selection = %v, want %v", got, tc.want)
			}
		})
	}
}

func selectionScore(v float64) *float64 { return &v }
