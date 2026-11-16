package analysis

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type classifierFunc func(context.Context, Pair) (Preclassification, error)

func (classifierFunc) Profile() string { return "laya-test-v1" }
func (f classifierFunc) Classify(ctx context.Context, p Pair) (Preclassification, error) {
	return f(ctx, p)
}
func relatedClassification() Preclassification {
	return Preclassification{Decision: "related", Scores: map[string]float64{"related": .8, "unrelated": .1, "uncertain": .1}}
}

func TestPipelineCommitsLayaBeforeLLMAndOverlapsNextPair(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	secondStarted, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	classifier := classifierFunc(func(ctx context.Context, p Pair) (Preclassification, error) {
		if calls.Add(1) == 2 {
			close(secondStarted)
			select {
			case <-release:
			case <-ctx.Done():
				return Preclassification{}, ctx.Err()
			}
		}
		return relatedClassification(), nil
	})
	entered := make(chan struct{}, 1)
	generator := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		var raw string
		if err := db.QueryRow(`SELECT laya_result::text FROM analysis_pairs WHERE left_source->>'id'=$1 AND right_source->>'id'=$2 AND laya_status='completed'`, p.Left.ID, p.Right.ID).Scan(&raw); err != nil {
			t.Errorf("LLM before committed Laya: %v", err)
		}
		if p.Preclassification == nil || p.Preclassification.Decision != "related" {
			t.Error("LLM did not receive persisted Laya snapshot")
		}
		select {
		case <-secondStarted:
		case <-ctx.Done():
			return Finding{}, ctx.Err()
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		return findingFor(p), nil
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	if _, err := s.Start(context.Background(), "a", "p"); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, entered)
}

func TestPipelineFailureIsPerPairAndRetryRetainsLaya(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	var classifications, generations atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	classifier := classifierFunc(func(context.Context, Pair) (Preclassification, error) {
		classifications.Add(1)
		return relatedClassification(), nil
	})
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) {
		generations.Add(1)
		if p.Left.ID == "requirement-0001" && p.Right.ID == "requirement-0002" && fail.Load() {
			return Finding{}, errors.New("failed")
		}
		return findingFor(p), nil
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, s, "a", "p", initial.ID)
	if done.FailedPairs != 1 || done.CompletedPairs != 2 || classifications.Load() != 3 || generations.Load() != 3 {
		t.Fatalf("failure blocked unrelated work: %+v classify=%d generate=%d", done, classifications.Load(), generations.Load())
	}
	fail.Store(false)
	if _, err = s.Retry(context.Background(), "a", "p", initial.ID); err != nil {
		t.Fatal(err)
	}
	done = waitForRun(t, s, "a", "p", initial.ID)
	if done.CompletedPairs != 3 || classifications.Load() != 3 || generations.Load() != 4 {
		t.Fatalf("retry repeated successful stage: %+v classify=%d generate=%d", done, classifications.Load(), generations.Load())
	}
}

func TestPipelineWaitReplacementPreservesInFlightLLM(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	llmStarted, layaRelease, llmRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var classifications atomic.Int32
	classifier := classifierFunc(func(ctx context.Context, p Pair) (Preclassification, error) {
		if classifications.Add(1) == 2 {
			select {
			case <-layaRelease:
			case <-ctx.Done():
				return Preclassification{}, ctx.Err()
			}
		}
		return relatedClassification(), nil
	})
	var generated atomic.Int32
	var cancelled atomic.Bool
	generator := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		if generated.Add(1) == 1 {
			close(llmStarted)
			select {
			case <-llmRelease:
			case <-ctx.Done():
				cancelled.Store(true)
				return Finding{}, ctx.Err()
			}
		}
		return findingFor(p), nil
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, llmStarted)
	if _, err = s.Replace(context.Background(), "a", "p", initial.ID, Options{Mode: "all", Threshold: .7}, "wait"); err != nil {
		t.Fatal(err)
	}
	close(layaRelease)
	// Wait for the Laya loop to encounter the replacement boundary.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !cancelled.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	close(llmRelease)
	if cancelled.Load() {
		t.Fatal("wait replacement cancelled current LLM when Laya reached its boundary")
	}
}

func TestPipelineLayaFailureStillRunsFinalLLMOnFullSource(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	var fail atomic.Bool
	fail.Store(true)
	var classified, generated atomic.Int32
	classifier := classifierFunc(func(_ context.Context, p Pair) (Preclassification, error) {
		classified.Add(1)
		if p.Left.ID == "requirement-0001" && p.Right.ID == "requirement-0002" && fail.Load() {
			return Preclassification{}, errors.New("local timeout")
		}
		decision := relatedClassification()
		decision.Decision = "unrelated"
		return decision, nil
	})
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) {
		generated.Add(1)
		if p.Left.Text == "" || p.Right.Text == "" {
			t.Error("final model did not receive full original sources")
		}
		if (p.Left.ID == "requirement-0001" && p.Right.ID == "requirement-0002") && p.Preclassification != nil {
			t.Error("failed Laya call was replaced with fabricated scores")
		}
		if p.Preclassification != nil && p.Preclassification.Decision != "unrelated" {
			t.Error("successful Laya suggestion changed")
		}
		return findingFor(p), nil
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, s, "a", "p", initial.ID)
	if done.CompletedPairs != 3 || done.LayaFailedPairs != 1 || generated.Load() != 3 || classified.Load() != 3 {
		t.Fatalf("Laya failure should remain visible while final LLM handles every pair: %+v classified=%d generated=%d", done, classified.Load(), generated.Load())
	}
	if done.Pairs[0].LayaStatus != "failed" || done.Pairs[0].LayaErrorCode != "LAYA_CLASSIFICATION_FAILED" || done.Pairs[0].Preclassification != nil {
		t.Fatalf("Laya failure diagnostics/scores were not preserved correctly: %+v", done.Pairs[0])
	}
}

func TestPipelineSkipsUnavailableLayaButRunsEveryFinalPair(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	var generated atomic.Int32
	classifier := classifierFunc(func(_ context.Context, p Pair) (Preclassification, error) {
		switch {
		case p.Left.ID == "requirement-0001" && p.Right.ID == "requirement-0002":
			return Preclassification{}, ErrClassificationInputTooLong
		case p.Left.ID == "requirement-0001" && p.Right.ID == "requirement-0003":
			return Preclassification{}, ErrSummaryUnavailable
		default:
			return Preclassification{}, errors.New("local provider failed")
		}
	})
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) {
		generated.Add(1)
		if p.Preclassification != nil || p.Left.Text == "" || p.Right.Text == "" {
			t.Error("fallback must pass full originals without invented classifier output")
		}
		return findingFor(p), nil
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, s, "a", "p", initial.ID)
	if done.Status != "completed" || done.CompletedPairs != 3 || done.LayaSkippedPairs != 2 || done.LayaFailedPairs != 1 || generated.Load() != 3 {
		t.Fatalf("Laya unavailability blocked final analysis: %+v generated=%d", done, generated.Load())
	}
	want := map[string]string{"requirement-0001:requirement-0002": "LAYA_INPUT_TOO_LONG", "requirement-0001:requirement-0003": "LAYA_SUMMARY_UNAVAILABLE", "requirement-0002:requirement-0003": "LAYA_CLASSIFICATION_FAILED"}
	for _, pair := range done.Pairs {
		key := pair.Left.ID + ":" + pair.Right.ID
		if pair.LayaErrorCode != want[key] || pair.Preclassification != nil {
			t.Fatalf("fallback diagnostics or scores incorrect for %s: %+v", key, pair)
		}
		if key == "requirement-0001:requirement-0002" || key == "requirement-0001:requirement-0003" {
			if pair.LayaStatus != "skipped" {
				t.Fatalf("expected skipped Laya stage for %s: %+v", key, pair)
			}
		} else if pair.LayaStatus != "failed" {
			t.Fatalf("expected failed Laya stage for %s: %+v", key, pair)
		}
	}
}

func TestPipelineRetryAfterFinalFailureRetainsLayaFailure(t *testing.T) {
	db, _, _ := analysisDB(t)
	classifierCalls, generatorCalls := atomic.Int32{}, atomic.Int32{}
	classifier := classifierFunc(func(context.Context, Pair) (Preclassification, error) {
		classifierCalls.Add(1)
		return Preclassification{}, ErrSummaryUnavailable
	})
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) {
		if generatorCalls.Add(1) == 1 {
			return Finding{}, errors.New("final model failed once")
		}
		return findingFor(p), nil
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForRun(t, s, "a", "p", initial.ID)
	if failed.FailedPairs != 1 || failed.Pairs[0].LayaStatus != "skipped" {
		t.Fatalf("expected final-model failure with retained Laya fallback: %+v", failed)
	}
	if _, err = s.Retry(context.Background(), "a", "p", initial.ID); err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, s, "a", "p", initial.ID)
	if done.CompletedPairs != 1 || classifierCalls.Load() != 1 || generatorCalls.Load() != 2 || done.Pairs[0].LayaStatus != "skipped" || done.Pairs[0].LayaErrorCode != "LAYA_SUMMARY_UNAVAILABLE" {
		t.Fatalf("retry repeated or lost Laya fallback state: %+v classify=%d generate=%d", done, classifierCalls.Load(), generatorCalls.Load())
	}
}

func TestPipelineRestartRetainsCommittedLaya(t *testing.T) {
	db, _, _ := analysisDB(t)
	var classified atomic.Int32
	classifier := classifierFunc(func(context.Context, Pair) (Preclassification, error) {
		classified.Add(1)
		return relatedClassification(), nil
	})
	entered := make(chan struct{})
	generator := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		close(entered)
		<-ctx.Done()
		return Finding{}, ctx.Err()
	})
	s := New(db, nil, generator, "", classifier)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	waitSignal(t, entered)
	s.Close()
	restored := New(db, nil, generatorFunc(func(_ context.Context, p Pair) (Finding, error) {
		if p.Preclassification == nil {
			t.Error("lost Laya snapshot")
		}
		return findingFor(p), nil
	}), "", classifier)
	t.Cleanup(restored.Close)
	if _, err = restored.Retry(context.Background(), "a", "p", initial.ID); err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, restored, "a", "p", initial.ID)
	if done.CompletedPairs != 1 || classified.Load() != 1 {
		t.Fatalf("restart repeated committed Laya: %+v calls=%d", done, classified.Load())
	}
}

func TestPipelineCancelFencesLateClassification(t *testing.T) {
	db, _, _ := analysisDB(t)
	entered, release := make(chan struct{}), make(chan struct{})
	classifier := classifierFunc(func(context.Context, Pair) (Preclassification, error) {
		close(entered)
		<-release
		return relatedClassification(), nil
	})
	var generated atomic.Int32
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) { generated.Add(1); return findingFor(p), nil })
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	waitSignal(t, entered)
	if _, err = s.Cancel(context.Background(), "a", "p", initial.ID); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	s.Close()
	done, err := s.Get(context.Background(), "a", "p", initial.ID)
	if err != nil || done.ErrorCode != "CANCELLED" || done.LayaCompletedPairs != 0 || generated.Load() != 0 {
		t.Fatalf("late answer accepted: %+v generated=%d err=%v", done, generated.Load(), err)
	}
}

func TestPipelineExactDuplicateKeepsDeterministicVerdict(t *testing.T) {
	db, _, _ := analysisDB(t)
	if _, err := db.Exec(`UPDATE requirements SET text='Same obligation' WHERE workspace='a' AND project_id='p'`); err != nil {
		t.Fatal(err)
	}
	classifier := classifierFunc(func(context.Context, Pair) (Preclassification, error) { return relatedClassification(), nil })
	var calls atomic.Int32
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) { calls.Add(1); return findingFor(p), nil })
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, s, "a", "p", initial.ID)
	if done.DuplicatePairs != 1 || calls.Load() != 0 || done.Pairs[0].ResultOrigin != exactOrigin {
		t.Fatalf("lost deterministic exact duplicate handling: %+v calls=%d", done, calls.Load())
	}
}

func TestPipelineCancellationCountsBothActiveStages(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	classifyStarted, generateStarted := make(chan struct{}), make(chan struct{})
	var classes atomic.Int32
	classifier := classifierFunc(func(ctx context.Context, p Pair) (Preclassification, error) {
		if classes.Add(1) == 2 {
			close(classifyStarted)
			<-ctx.Done()
			return Preclassification{}, ctx.Err()
		}
		return relatedClassification(), nil
	})
	generator := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		close(generateStarted)
		<-ctx.Done()
		return Finding{}, ctx.Err()
	})
	s := New(db, nil, generator, "", classifier)
	t.Cleanup(s.Close)
	initial, err := s.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, classifyStarted)
	waitSignal(t, generateStarted)
	cancelled, err := s.Cancel(context.Background(), "a", "p", initial.ID)
	if err != nil || cancelled.ProcessedPairs != 2 || cancelled.LayaFailedPairs != 2 || cancelled.LayaPendingPairs != 0 {
		t.Fatalf("two active stages counted as one: processed=%d err=%v", cancelled.ProcessedPairs, err)
	}
}

func TestPipelineSearchDoesNotWaitForEitherModel(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	if _, err := db.Exec(`INSERT INTO requirements VALUES('third','a','p','Third obligation',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	layaStarted, llmStarted := make(chan struct{}), make(chan struct{})
	var classes atomic.Int32
	classifier := classifierFunc(func(ctx context.Context, p Pair) (Preclassification, error) {
		if classes.Add(1) == 2 {
			close(layaStarted)
			<-ctx.Done()
			return Preclassification{}, ctx.Err()
		}
		return relatedClassification(), nil
	})
	generator := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		close(llmStarted)
		<-ctx.Done()
		return Finding{}, ctx.Err()
	})
	s := New(db, retriever, generator, "embed-a", classifier)
	t.Cleanup(s.Close)
	if _, err := s.Start(context.Background(), "a", "p"); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, layaStarted)
	waitSignal(t, llmStarted)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	hits, err := retriever.Search(ctx, "a", "p", "Оплата", 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search blocked behind analysis: hits=%d err=%v", len(hits), err)
	}
}
