package analysis

import (
	"context"
	"database/sql"
	"errors"
	"github.com/VolkovDani/project-creator/internal/rag"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartReturnsBeforeModelCompletes(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	gen := &fakeGenerator{profile: "gen-a", started: make(chan struct{}, 1), release: make(chan struct{})}
	service := New(db, retriever, gen, "embed-a")
	t.Cleanup(service.Close)
	defer close(gen.release)
	result := make(chan Run, 1)
	errors := make(chan error, 1)
	go func() {
		run, err := service.Start(context.Background(), "a", "p")
		result <- run
		errors <- err
	}()
	select {
	case run := <-result:
		if err := <-errors; err != nil || run.Status != "running" {
			t.Fatalf("initial state must be durable running: %+v, %v", run, err)
		}
		if run.CreatedAt == "" || run.Stage != "checking_index" || run.TotalPairs != nil {
			t.Fatalf("initial snapshot lacks persisted metadata or invents a pair total: %+v", run)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Start holds the caller until the model completes")
	}
}

func addThird(t *testing.T, db *sql.DB, retriever *rag.Service) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO requirements VALUES('middle','a','p','Покупатель получает чек.',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	state, err := retriever.Index(context.Background(), "a", "p", "middle")
	if err != nil || state.Status != "ready" {
		t.Fatal(state, err)
	}
}
func findingFor(pair Pair) Finding {
	return Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "compatible", Explanation: "Нет противоречия в этой паре."}
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("model did not reach expected pair")
	}
}

func TestProgressPersistsBeforeRemainingPairsAndSurvivesRequestCancellation(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	var calls atomic.Int32
	second := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	generator := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		if calls.Add(1) == 2 {
			close(second)
			select {
			case <-release:
			case <-ctx.Done():
				return Finding{}, ctx.Err()
			}
		}
		return findingFor(pair), nil
	})
	service := testService(t, db, retriever, generator, "embed-a")
	ctx, cancel := context.WithCancel(context.Background())
	run, err := service.Start(ctx, "a", "p")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, second)
	progress, err := service.Latest(context.Background(), "a", "p")
	if err != nil || progress.ID != run.ID || progress.Stage != "comparing" || progress.TotalPairs == nil || *progress.TotalPairs != 3 || progress.ProcessedPairs != 1 || len(progress.Findings) != 1 {
		t.Fatalf("partial progress: %+v %v", progress, err)
	}
	if progress.Pairs[0].Status != "completed" || progress.Pairs[1].Status != "running" || progress.Pairs[2].Status != "pending" || progress.ActivePairID != progress.Pairs[1].ID {
		t.Fatal(progress.Pairs)
	}
	persisted, err := service.Get(context.Background(), "a", "p", run.ID)
	if err != nil || len(persisted.Findings) != 1 || persisted.ProcessedPairs != 1 {
		t.Fatalf("not durable before next pair: %+v %v", persisted, err)
	}
}

func TestPairFailureRetryKeepsCompletedPairsAndRequiresCurrentLatestSnapshot(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	var fail atomic.Bool
	fail.Store(true)
	var calls atomic.Int32
	var mu sync.Mutex
	byPair := map[string]int{}
	generator := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		mu.Lock()
		byPair[pair.Left.ID+"/"+pair.Right.ID]++
		mu.Unlock()
		if calls.Add(1) == 2 && fail.Load() {
			return Finding{}, errors.New("provider stopped")
		}
		return findingFor(pair), nil
	})
	service := testService(t, db, retriever, generator, "embed-a")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.Status != "failed" || run.ErrorCode != "GENERATION_FAILED" || len(run.Findings) != 1 || run.ProcessedPairs != 2 || len(run.Pairs) != 3 {
		t.Fatalf("failed batch: %+v %v", run, err)
	}
	if run.Pairs[0].Status != "completed" || run.Pairs[1].ErrorCode != "GENERATION_FAILED" || run.Pairs[2].ErrorCode != "NOT_PROCESSED" {
		t.Fatal(run.Pairs)
	}
	if _, err = service.Retry(context.Background(), "b", "p", run.ID); err != ErrNotFound {
		t.Fatalf("cross-workspace retry: %v", err)
	}
	fail.Store(false)
	resumed, err := service.Retry(context.Background(), "a", "p", run.ID)
	if err != nil || resumed.ID != run.ID || resumed.ProcessedPairs != 1 {
		t.Fatalf("retry: %+v %v", resumed, err)
	}
	done := waitForRun(t, service, "a", "p", run.ID)
	if done.Status != "completed" || len(done.Findings) != 3 || done.ProcessedPairs != 3 || done.Pairs[0].ID != run.Pairs[0].ID {
		t.Fatal(done)
	}
	mu.Lock()
	firstCalls := byPair[run.Pairs[0].Left.ID+"/"+run.Pairs[0].Right.ID]
	mu.Unlock()
	if firstCalls != 1 || calls.Load() != 4 {
		t.Fatalf("successful pair repeated: first=%d all=%d", firstCalls, calls.Load())
	}
	newer, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Retry(context.Background(), "a", "p", run.ID); err != ErrRetryUnavailable {
		t.Fatalf("old run hidden behind %s must not resume: %v", newer.ID, err)
	}
}

func TestInvalidPairOutputContinuesQueueAndRetriesOnlyRejectedPair(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	var calls atomic.Int32
	retryPair := make(chan Pair, 1)
	generator := generatorFunc(func(_ context.Context, p Pair) (Finding, error) {
		switch calls.Add(1) {
		case 2:
			return Finding{}, npuFailure("INVALID_OUTPUT")
		case 4:
			retryPair <- p
		}
		return findingFor(p), nil
	})
	service := testService(t, db, retriever, generator, "embed-a")
	ctx := context.Background()
	run, err := service.startAndWait(t, ctx, "a", "p")
	if err != nil || run.Status != "failed" || run.ErrorCode != "INVALID_OUTPUT" ||
		run.CompletedPairs != 2 || run.FailedPairs != 1 || run.ProcessedPairs != 3 || len(run.Pairs) != 3 {
		t.Fatalf("status=%s code=%s completed=%d failed=%d processed=%d err=%v",
			run.Status, run.ErrorCode, run.CompletedPairs, run.FailedPairs, run.ProcessedPairs, err)
	}
	rejected := run.Pairs[1]
	if run.Pairs[0].Status != "completed" || run.Pairs[2].Status != "completed" ||
		rejected.Status != "failed" || rejected.ErrorCode != "INVALID_OUTPUT" || rejected.Verdict != "" || rejected.Explanation != "" {
		t.Fatal(run.Pairs)
	}
	if _, err = service.Retry(ctx, "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, service, "a", "p", run.ID)
	if done.Status != "completed" || done.CompletedPairs != 3 || done.FailedPairs != 0 ||
		calls.Load() != 4 {
		t.Fatalf("retry status=%s completed=%d failed=%d calls=%d", done.Status, done.CompletedPairs, done.FailedPairs, calls.Load())
	}
	if retried := <-retryPair; retried.Left.ID != rejected.Left.ID || retried.Right.ID != rejected.Right.ID {
		t.Fatalf("retried the wrong pair: %v", retried)
	}
}

func TestRetryRejectsChangedSourcesAndProfiles(t *testing.T) {
	for _, change := range []string{"sources", "generator"} {
		t.Run(change, func(t *testing.T) {
			db, retriever, _ := analysisDB(t)
			ready(t, retriever)
			gen := &fakeGenerator{profile: "gen-a", fail: true}
			service := testService(t, db, retriever, gen, "embed-a")
			run, err := service.startAndWait(t, context.Background(), "a", "p")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "sources":
				_, err = db.Exec(`UPDATE requirements SET text='Changed',version=2 WHERE id='left'`)
			case "generator":
				service = testService(t, db, retriever, &fakeGenerator{profile: "gen-b"}, "embed-a")
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Retry(context.Background(), "a", "p", run.ID); err != ErrRetryUnavailable {
				t.Fatalf("retry reused changed %s: %v", change, err)
			}
		})
	}
}

func TestShutdownPreservesPartialPairsAndRestartRetriesOnlyInterruptedWork(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	var calls atomic.Int32
	second := make(chan struct{})
	generator := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		if calls.Add(1) == 2 {
			close(second)
			<-ctx.Done()
			return Finding{}, ctx.Err()
		}
		return findingFor(pair), nil
	})
	service := testService(t, db, retriever, generator, "embed-a")
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, second)
	service.Close()
	var remaining atomic.Int32
	replacement := testService(t, db, retriever, generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) { remaining.Add(1); return findingFor(pair), nil }), "embed-a")
	interrupted, err := replacement.Latest(context.Background(), "a", "p")
	if err != nil || interrupted.ID != run.ID || interrupted.Status != "failed" || interrupted.ErrorCode != "INTERRUPTED" || len(interrupted.Findings) != 1 || interrupted.Pairs[1].ErrorCode != "INTERRUPTED" || interrupted.Pairs[2].ErrorCode != "NOT_PROCESSED" {
		t.Fatalf("restart state: %+v %v", interrupted, err)
	}
	if _, err = replacement.Retry(context.Background(), "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitForRun(t, replacement, "a", "p", run.ID)
	if done.Status != "completed" || len(done.Findings) != 3 || remaining.Load() != 2 {
		t.Fatalf("restart duplicated completed work: %+v calls=%d", done, remaining.Load())
	}
}

func TestMultipleServicesAndProjectsSerializeModelCalls(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	if _, err := db.Exec(`INSERT INTO requirements VALUES('foreign-2','a','q','Second foreign requirement',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"foreign", "foreign-2"} {
		state, err := retriever.Index(context.Background(), "a", "q", id)
		if err != nil || state.Status != "ready" {
			t.Fatal(state, err)
		}
	}
	var active atomic.Int32
	var maximum atomic.Int32
	entered := make(chan struct{}, 4)
	release := make(chan struct{}, 4)
	generator := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return Finding{}, ctx.Err()
		}
		return findingFor(pair), nil
	})
	first := testService(t, db, retriever, generator, "embed-a")
	second := testService(t, db, retriever, generator, "embed-a")
	a, err := first.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, entered)
	b, err := second.Start(context.Background(), "a", "q")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
		t.Fatal("second backend called the model concurrently")
	case <-time.After(150 * time.Millisecond):
	}
	release <- struct{}{}
	waitSignal(t, entered)
	release <- struct{}{}
	if waitForRun(t, first, "a", "p", a.ID).Status != "completed" || waitForRun(t, second, "a", "q", b.ID).Status != "completed" || maximum.Load() != 1 {
		t.Fatalf("concurrent generation: %d", maximum.Load())
	}
}

func TestQueuedProfileDriftNoForeverRunning(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	// A persisted queue item from a process that exited before claiming it.
	queued := &Service{db: db, retriever: retriever, generator: &fakeGenerator{profile: "old"}, embeddingProfile: "embed-a"}
	run, err := queued.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	current := testService(t, db, retriever, &fakeGenerator{profile: "new"}, "embed-a")
	deadline := time.Now().Add(5 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		if err = db.QueryRow(`SELECT status FROM analysis_runs WHERE id=$1`, run.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "failed" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if status != "failed" {
		t.Fatal("background recovery left an old-profile queue item running")
	}
	latest, err := current.Latest(context.Background(), "a", "p")
	if err != nil || latest.ID != run.ID || latest.Status != "failed" || latest.ErrorCode != "PROFILE_CHANGED" || !latest.Stale {
		t.Fatalf("queued old profile stayed running: %+v %v", latest, err)
	}
}

func TestLossOfModelLockSessionCancelsGeneration(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	started := make(chan struct{})
	service := testService(t, db, retriever, generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		close(started)
		<-ctx.Done()
		return Finding{}, ctx.Err()
	}), "embed-a")
	service.heartbeatInterval = 20 * time.Millisecond
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started)
	var pid int
	if err = db.QueryRow(`SELECT pid FROM pg_locks WHERE locktype='advisory' AND database=(SELECT oid FROM pg_database WHERE datname=current_database()) AND classid=0 AND objid=$1 AND objsubid=1 AND granted`, modelLock).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var terminated bool
	if err = db.QueryRow(`SELECT pg_terminate_backend($1)`, pid).Scan(&terminated); err != nil || !terminated {
		t.Fatal("terminate isolated test lock session", err)
	}
	interrupted := waitForRun(t, service, "a", "p", run.ID)
	if interrupted.Status != "failed" || interrupted.ErrorCode != "INTERRUPTED" || interrupted.Pairs[0].Status != "failed" {
		t.Fatalf("lost lock left inference running: %+v", interrupted)
	}
}

func TestCancelFencesLatePairAndIsIdempotent(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var calls atomic.Int32
	gen := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		if calls.Add(1) == 2 {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release // Deliberately return a valid late response after cancellation.
		}
		return findingFor(pair), nil
	})
	service := testService(t, db, retriever, gen, "embed-a")
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started)
	canceled, err := service.Cancel(context.Background(), "a", "p", run.ID)
	if err != nil || canceled.Status != "failed" || canceled.ErrorCode != "CANCELLED" || canceled.ProcessedPairs != 2 || canceled.Pairs[0].Status != "completed" || canceled.Pairs[1].ErrorCode != "CANCELLED" || canceled.Pairs[2].ErrorCode != "CANCELLED" {
		t.Fatalf("cancel state: %+v %v", canceled, err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("active model call did not receive cancellation promptly")
	}
	releaseOnce.Do(func() { close(release) })
	time.Sleep(50 * time.Millisecond)
	after, err := service.Get(context.Background(), "a", "p", run.ID)
	if err != nil || after.Status != "failed" || after.ErrorCode != "CANCELLED" || after.CompletedPairs != 1 || calls.Load() != 2 {
		t.Fatalf("late result escaped cancellation fence: %+v calls=%d err=%v", after, calls.Load(), err)
	}
	again, err := service.Cancel(context.Background(), "a", "p", run.ID)
	if err != nil || again.ErrorCode != "CANCELLED" {
		t.Fatalf("repeat cancel: %+v %v", again, err)
	}
	if _, err = service.Cancel(context.Background(), "b", "p", run.ID); err != ErrNotFound {
		t.Fatalf("cross-workspace cancel: %v", err)
	}
	if _, err = service.Cancel(context.Background(), "a", "q", run.ID); err != ErrNotFound {
		t.Fatalf("cross-project cancel: %v", err)
	}
	if _, err = service.Retry(context.Background(), "a", "p", run.ID); err != nil {
		t.Fatalf("retry cancelled run: %v", err)
	}
	completed := waitForRun(t, service, "a", "p", run.ID)
	if completed.Status != "completed" || completed.CompletedPairs != 3 || calls.Load() != 4 {
		t.Fatalf("retry did not preserve completed pair: %+v calls=%d", completed, calls.Load())
	}
	unchanged, err := service.Cancel(context.Background(), "a", "p", run.ID)
	if err != nil || unchanged.Status != "completed" || unchanged.CompletedPairs != 3 {
		t.Fatalf("cancel changed a completed run: %+v %v", unchanged, err)
	}
}

func TestCancelQueuedRun(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	service := &Service{db: db, retriever: retriever, generator: &fakeGenerator{profile: "gen-a"}, embeddingProfile: "embed-a"}
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	canceled, err := service.Cancel(context.Background(), "a", "p", run.ID)
	if err != nil || canceled.Status != "failed" || canceled.ErrorCode != "CANCELLED" {
		t.Fatalf("queued cancel: %+v %v", canceled, err)
	}
}

func TestLostLockDoesNotOverlapAnotherServiceDuringProviderTeardown(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	if _, err := db.Exec(`INSERT INTO requirements VALUES('foreign-2','a','q','Second foreign requirement',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"foreign", "foreign-2"} {
		if _, err := retriever.Index(context.Background(), "a", "q", id); err != nil {
			t.Fatal(err)
		}
	}
	var active, maximum, calls atomic.Int32
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	generator := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) {
				break
			}
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-release
			return Finding{}, ctx.Err()
		}
		return findingFor(pair), nil
	})
	first := testService(t, db, retriever, generator, "embed-a")
	first.heartbeatInterval = 50 * time.Millisecond
	second := testService(t, db, retriever, generator, "embed-a")
	second.heartbeatInterval = 50 * time.Millisecond
	a, err := first.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, entered)
	b, err := second.Start(context.Background(), "a", "q")
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err = db.QueryRow(`SELECT pid FROM pg_locks WHERE locktype='advisory' AND database=(SELECT oid FROM pg_database WHERE datname=current_database()) AND classid=0 AND objid=$1 AND objsubid=1 AND granted`, modelLock).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var terminated bool
	if err = db.QueryRow(`SELECT pg_terminate_backend($1)`, pid).Scan(&terminated); err != nil || !terminated {
		t.Fatal(err)
	}
	waitSignal(t, cancelled)
	// Cancellation is not proof that an adapter finished native cleanup.
	time.Sleep(150 * time.Millisecond)
	if maximum.Load() != 1 || calls.Load() != 1 {
		t.Fatalf("model calls overlapped while old Compare was still returning: max=%d calls=%d", maximum.Load(), calls.Load())
	}
	releaseOnce.Do(func() { close(release) })
	if firstDone := waitForRun(t, first, "a", "p", a.ID); firstDone.ErrorCode != "INTERRUPTED" {
		t.Fatal(firstDone)
	}
	if secondDone := waitForRun(t, second, "a", "q", b.ID); secondDone.Status != "completed" || maximum.Load() != 1 {
		t.Fatal(secondDone, maximum.Load())
	}
}
