package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type summaryQualityCase struct {
	Name            string   `json:"name"`
	Language        string   `json:"language"`
	Source          string   `json:"source"`
	Facts           []string `json:"facts"`
	IdentifierCount int      `json:"identifierCount"`
}

func loadSummaryQualityCases(t *testing.T) []summaryQualityCase {
	t.Helper()
	data, err := os.ReadFile("testdata/summary-quality.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []summaryQualityCase
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func summaryQualitySource(tc summaryQualityCase) string {
	if tc.IdentifierCount == 0 {
		return tc.Source
	}
	ids := make([]string, tc.IdentifierCount)
	for i := range ids {
		ids[i] = fmt.Sprintf("SEQ-%04d", i+1)
	}
	return tc.Source + strings.Join(ids, ", ")
}

// This conservative lexical check is a smoke signal, not a semantic proof.
func summaryContainsFacts(summary string, facts []string) []string {
	missing := []string{}
	for _, fact := range facts {
		if !strings.Contains(strings.ToLower(summary), strings.ToLower(fact)) {
			missing = append(missing, fact)
		}
	}
	return missing
}

func TestSummaryQualityEvaluatorDetectsDroppedCriticalFact(t *testing.T) {
	missing := summaryContainsFacts("The Owner must keep records for 90 days.", []string{"Owner", "90 days", "Viewer must never delete"})
	if len(missing) != 1 || missing[0] != "Viewer must never delete" {
		t.Fatalf("missing facts=%v", missing)
	}
}

// Opt in only with a locally configured model. It deliberately uses synthetic
// fixtures and reports source/summary pairs for human review, not model scores.
func TestSummaryQualitySmoke(t *testing.T) {
	url := os.Getenv("SUMMARY_SMOKE_URL")
	if url == "" {
		t.Skip("set SUMMARY_SMOKE_URL, SUMMARY_SMOKE_MODEL and SUMMARY_SMOKE_DIGEST to run local quality smoke")
	}
	model, digest := os.Getenv("SUMMARY_SMOKE_MODEL"), os.Getenv("SUMMARY_SMOKE_DIGEST")
	if model == "" || digest == "" {
		t.Fatal("SUMMARY_SMOKE_MODEL and SUMMARY_SMOKE_DIGEST are required when SUMMARY_SMOKE_URL is set")
	}
	g, err := NewSummaryOllama(url, model, digest)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range loadSummaryQualityCases(t) {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			source := summaryQualitySource(tc)
			if len(source) <= 350 {
				t.Fatalf("fixture source is not longer than 350 bytes: %d", len(source))
			}
			got, callErr := g.Summarize(context.Background(), Source{ID: tc.Name, Version: 1, Text: source})
			t.Logf("model=%s profile=%s input_bytes=%d summary_bytes=%d input=%q summary=%q", model, g.Profile(), len(source), len(got), source, got)
			if tc.IdentifierCount > 0 {
				if callErr != ErrSummaryImpossible {
					t.Fatalf("expected impossible-to-compress refusal, got summary=%q err=%v", got, callErr)
				}
				return
			}
			if callErr != nil {
				t.Fatal(callErr)
			}
			if missing := summaryContainsFacts(got, tc.Facts); len(missing) > 0 {
				t.Errorf("summary omitted expected critical facts %q", missing)
			}
		})
	}
}
