package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/VolkovDani/project-creator/internal/rag"
)

func selectionVector(component int) string {
	values := make([]string, 1024)
	for i := range values {
		values[i] = "0"
	}
	values[component] = "1"
	return "[" + strings.Join(values, ",") + "]"
}

func selectionFixtures(t *testing.T, db *sql.DB, retriever *rag.Service, texts []string) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND project_id='p'`); err != nil {
		t.Fatal(err)
	}
	for i, text := range texts {
		id := fmt.Sprintf("fixture-%02d", i)
		if _, err := db.Exec(`INSERT INTO requirements VALUES($1,'a','p',$2,1,'now','now')`, id, text); err != nil {
			t.Fatal(err)
		}
		state, err := retriever.Index(context.Background(), "a", "p", id)
		if err != nil || state.Status != "ready" {
			t.Fatalf("fixture %s index: %+v %v", id, state, err)
		}
	}
}

func relatedRun(t *testing.T, service *Service, threshold float64) Run {
	t.Helper()
	run, err := service.StartWithOptions(context.Background(), "a", "p", Options{Mode: "related", Threshold: threshold})
	if err != nil {
		t.Fatal(err)
	}
	return waitForRun(t, service, "a", "p", run.ID)
}

func hasSelectionReason(pair PairResult, reason string) bool {
	for _, actual := range pair.SelectionReasons {
		if actual == reason {
			return true
		}
	}
	return false
}

func TestRelatedQueueFixedRUENCoverageBeyondTopSix(t *testing.T) {
	db, retriever, embed := analysisDB(t)
	texts := []string{
		"Оплата производится банковской картой.",
		"Покупатель вносит деньги наличными.",
		"Payment requires a credit card.",
		"Customer settles invoices electronically.",
		"Магазин принимает переводы.",
		"Checkout supports debit transactions.",
		"Касса обрабатывает расчёты.",
		"Billing confirms successful purchases.",
		"Weather measurements record wind direction.",
		"Архив хранит фотографии растений.",
	}
	selectionFixtures(t, db, retriever, texts)
	for _, noise := range []struct {
		id        string
		component int
	}{{"fixture-08", 1}, {"fixture-09", 2}} {
		if _, err := db.Exec(`UPDATE requirement_chunks SET embedding=$1::public.vector WHERE requirement_id=$2`, selectionVector(noise.component), noise.id); err != nil {
			t.Fatal(err)
		}
	}
	embeddingCalls := embed.calls.Load()
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, retriever, generator, "embed-a")
	run := relatedRun(t, service, 0.65)
	if run.Status != "completed" || run.PossiblePairs == nil || *run.PossiblePairs != 45 || run.TotalPairs == nil || *run.TotalPairs != 28 || run.NotSelectedPairs == nil || *run.NotSelectedPairs != 17 || generator.calls.Load() != 28 || run.CollisionPairs != 1 || run.SelectionProfile == "" || run.Mode != "related" || run.SimilarityThreshold != 0.65 {
		t.Fatalf("fixed RU/EN selection must include seven neighbors per source and exclude orthogonal noise: %+v calls=%d", run, generator.calls.Load())
	}
	pairs, err := service.Pairs(context.Background(), "a", "p", run.ID, 100, 0, "", "")
	if err != nil || len(pairs) != 28 {
		t.Fatalf("all semantic candidates: %+v %v", pairs, err)
	}
	neighbors := map[string]int{}
	for _, pair := range pairs {
		if pair.Left.ID == "fixture-08" || pair.Right.ID == "fixture-08" || pair.Left.ID == "fixture-09" || pair.Right.ID == "fixture-09" || !hasSelectionReason(pair, "semantic_similarity") || pair.SimilarityScore == nil || *pair.SimilarityScore < 0.999 {
			t.Fatalf("wrong semantic candidate/provenance: %+v", pair)
		}
		neighbors[pair.Left.ID]++
		neighbors[pair.Right.ID]++
	}
	if len(neighbors) != 8 {
		t.Fatalf("lost selected source: %+v", neighbors)
	}
	for id, count := range neighbors {
		if count != 7 {
			t.Fatalf("top-k clipped %s to %d neighbors", id, count)
		}
	}
	repeated := relatedRun(t, service, 0.65)
	if repeated.ReusedPairs != 28 || repeated.NewPairs != 0 || generator.calls.Load() != 28 || embed.calls.Load() != embeddingCalls {
		t.Fatalf("repeat performed inference or embedding: %+v inference=%d embedding=%d", repeated, generator.calls.Load(), embed.calls.Load())
	}
}

func TestRelatedQueueSharedTermsCannotBypassThreshold(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	selectionFixtures(t, db, retriever, []string{
		"Invoice retention lasts thirty days.",
		"Invoice retention ends after sixty days.",
		"Weather measurements record wind direction.",
	})
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`UPDATE requirement_chunks SET embedding=$1::public.vector WHERE requirement_id=$2`, selectionVector(i), fmt.Sprintf("fixture-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, retriever, generator, "embed-a")
	run := relatedRun(t, service, 0.65)
	pairs, err := service.Pairs(context.Background(), "a", "p", run.ID, 100, 0, "", "")
	if err != nil || run.TotalPairs == nil || *run.TotalPairs != 0 || run.NotSelectedPairs == nil || *run.NotSelectedPairs != 3 || len(pairs) != 0 || generator.calls.Load() != 0 {
		t.Fatalf("shared terms bypassed semantic threshold: run=%+v pairs=%+v err=%v", run, pairs, err)
	}
}

func TestRelatedQueueThresholdChangesRebuildSelectionButReuseComparison(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	selectionFixtures(t, db, retriever, []string{"Payments require cards.", "Покупатель вносит наличные."})
	values := make([]string, 1024)
	for i := range values {
		values[i] = "0"
	}
	values[0] = "0.8"
	values[1] = "0.6"
	if _, err := db.Exec(`UPDATE requirement_chunks SET embedding=$1::public.vector WHERE requirement_id='fixture-01'`, "["+strings.Join(values, ",")+"]"); err != nil {
		t.Fatal(err)
	}
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, retriever, generator, "embed-a")
	high := relatedRun(t, service, 0.9)
	if high.TotalPairs == nil || *high.TotalPairs != 0 || high.NotSelectedPairs == nil || *high.NotSelectedPairs != 1 || generator.calls.Load() != 0 {
		t.Fatalf("high threshold selected unrelated candidate: %+v", high)
	}
	selected := relatedRun(t, service, 0.65)
	if selected.TotalPairs == nil || *selected.TotalPairs != 1 || generator.calls.Load() != 1 {
		t.Fatalf("lower threshold did not select exact cosine candidate: %+v", selected)
	}
	rebuilt := relatedRun(t, service, 0.7)
	if rebuilt.ReusedPairs != 1 || rebuilt.NewPairs != 0 || generator.calls.Load() != 1 || rebuilt.SelectionProfile == selected.SelectionProfile {
		t.Fatalf("selection identity contaminated comparison identity: %+v", rebuilt)
	}
}

func TestExactDuplicatesNormalizeWhitespaceWithoutInference(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	selectionFixtures(t, db, retriever, []string{"Payment must be recorded.", "  Payment\t must\nbe   recorded. ", "Payment must be recorded.\n"})
	generator := &fakeGenerator{profile: "profile", fail: true}
	service := testService(t, db, nil, generator, "")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.DuplicatePairs != 3 || run.CompletedPairs != 3 || generator.calls.Load() != 0 {
		t.Fatalf("exact duplicate performed inference: %+v calls=%d err=%v", run, generator.calls.Load(), err)
	}
	pairs, err := service.Pairs(context.Background(), "a", "p", run.ID, 100, 0, "completed", "duplicate")
	if err != nil || len(pairs) != 3 {
		t.Fatalf("duplicate group: %+v %v", pairs, err)
	}
	for _, pair := range pairs {
		if pair.ResultOrigin != "exact_text_v1" || pair.CheckedAt == "" || pair.Explanation != "analysis.exactDuplicateExplanation" || pair.Left.Text == pair.Right.Text {
			t.Fatalf("exact duplicate lost algorithm identity or original text: %+v", pair)
		}
	}
}

func TestExactDuplicateDoesNotEraseNegationNumbersCaseOrPunctuation(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	selectionFixtures(t, db, retriever, []string{
		"Payment must be recorded.", "Payment must not be recorded.",
		"Delete records after 30 days.", "Delete records after 60 days.",
		"Case matters.", "case matters.", "Retry?", "Retry.",
	})
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, nil, generator, "")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.DuplicatePairs != 0 || generator.calls.Load() != 28 {
		t.Fatalf("normalization changed source meaning: %+v calls=%d err=%v", run, generator.calls.Load(), err)
	}
}

func TestSemanticDuplicateIsModelSuggestionAndReusable(t *testing.T) {
	db, _, _ := analysisDB(t)
	var calls atomic.Int32
	generator := generatorFunc(func(_ context.Context, pair Pair) (Finding, error) {
		calls.Add(1)
		finding := findingFor(pair)
		finding.Verdict = "duplicate"
		return finding, nil
	})
	service := testService(t, db, nil, generator, "")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.DuplicatePairs != 1 {
		t.Fatalf("semantic duplicate rejected: %+v %v", run, err)
	}
	pairs, err := service.Pairs(context.Background(), "a", "p", run.ID, 100, 0, "completed", "duplicate")
	if err != nil || len(pairs) != 1 || pairs[0].ResultOrigin != "model" {
		t.Fatalf("model duplicate presented as exact algorithm result: %+v %v", pairs, err)
	}
	reused, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || reused.ReusedPairs != 1 || reused.NewPairs != 0 || reused.DuplicatePairs != 1 || calls.Load() != 1 {
		t.Fatalf("semantic duplicate not reusable: %+v %v", reused, err)
	}
}

func TestRelatedExactRUDuplicatesSelectedWithoutCosineOrInference(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	selectionFixtures(t, db, retriever, []string{"Все гости видят отчёты.", " Все\tгости\n видят  отчёты. ", "Weather balloons measure atmospheric pressure."})
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`UPDATE requirement_chunks SET embedding=$1::public.vector WHERE requirement_id=$2`, selectionVector(i), fmt.Sprintf("fixture-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	generator := &fakeGenerator{profile: "profile", fail: true}
	service := testService(t, db, retriever, generator, "embed-a")
	run := relatedRun(t, service, 1)
	pairs, err := service.Pairs(context.Background(), "a", "p", run.ID, 100, 0, "", "")
	if err != nil || run.Status != "completed" || run.DuplicatePairs != 1 || generator.calls.Load() != 0 || len(pairs) != 1 || !hasSelectionReason(pairs[0], "exact_text") || pairs[0].ResultOrigin != "exact_text_v1" {
		t.Fatalf("RU exact duplicate omitted or sent to model: run=%+v pairs=%+v err=%v", run, pairs, err)
	}
}

func TestRelatedSelectionRejectsInvalidOptionsBeforeModelCalls(t *testing.T) {
	db, _, _ := analysisDB(t)
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, nil, generator, "")
	for _, options := range []Options{{Mode: "unknown", Threshold: 0.65}, {Mode: "related", Threshold: -0.1}, {Mode: "related", Threshold: 1.1}, {Mode: "related", Threshold: math.NaN()}, {Mode: "related", Threshold: math.Inf(1)}} {
		if _, err := service.StartWithOptions(context.Background(), "a", "p", options); err == nil {
			t.Fatalf("invalid selection options accepted: %+v", options)
		}
	}
	if generator.calls.Load() != 0 {
		t.Fatal("invalid selection invoked provider")
	}
}
