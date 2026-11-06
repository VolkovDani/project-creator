package analysis

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestReplacementWaitPreservesCurrentPairAndCachesIt(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	gen := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return Finding{}, ctx.Err()
			}
		}
		return findingFor(p), nil
	})
	service := testService(t, db, retriever, gen, "embed-a")
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	old, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, entered)
	opts := Options{Mode: "all", Threshold: .55}
	if _, err = service.Replace(context.Background(), "b", "p", old.ID, opts, "wait"); err != ErrNotFound {
		t.Fatalf("scope: %v", err)
	}
	for i := 0; i < 2; i++ {
		r, e := service.Replace(context.Background(), "a", "p", old.ID, opts, "wait")
		if e != nil || r.Stage != "waiting_current_pair" {
			t.Fatalf("replace: %+v %v", r, e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("wait issued more work")
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		latest, e := service.Latest(context.Background(), "a", "p")
		if e != nil {
			t.Fatal(e)
		}
		if latest.ID != old.ID && latest.Status == "completed" {
			prior, e := service.Get(context.Background(), "a", "p", old.ID)
			if e != nil || prior.CompletedPairs != 1 || prior.ErrorCode != "REPLACED" || latest.ReusedPairs != 1 || calls.Load() != 3 {
				t.Fatalf("prior %+v latest %+v calls %d err %v", prior, latest, calls.Load(), e)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("replacement never finished")
}

func TestReplacementCancelFencesLateResponseAndIsIdempotent(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls, active, maximum atomic.Int32
	gen := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		n := active.Add(1)
		defer active.Add(-1)
		if n > maximum.Load() {
			maximum.Store(n)
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-release
			return findingFor(p), nil
		}
		return findingFor(p), nil
	})
	s := testService(t, db, retriever, gen, "embed-a")
	other := testService(t, db, retriever, gen, "embed-a")
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	old, e := s.Start(context.Background(), "a", "p")
	if e != nil {
		t.Fatal(e)
	}
	waitSignal(t, entered)
	opts := Options{Mode: "all", Threshold: .55}
	for i := 0; i < 2; i++ {
		if _, e = other.Replace(context.Background(), "a", "p", old.ID, opts, "cancel"); e != nil {
			t.Fatal(e)
		}
	}
	// Other service observes cancellation via durable lease fencing/heartbeat.
	select {
	case <-cancelled:
	case <-time.After(7 * time.Second):
		t.Fatal("remote worker did not observe cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("successor ran while previous inference was alive")
	}
	close(release)
	next := waitForReplacement(t, s, old.ID)
	prior, e := s.Get(context.Background(), "a", "p", old.ID)
	if e != nil || prior.CompletedPairs != 0 || next.ReusedPairs != 0 || next.CompletedPairs != 3 || maximum.Load() != 1 {
		t.Fatalf("old %+v new %+v max %d err %v", prior, next, maximum.Load(), e)
	}
	repeated, e := s.Replace(context.Background(), "a", "p", old.ID, opts, "cancel")
	if e != nil || repeated.ID != next.ID {
		t.Fatalf("idempotency %+v %v", repeated, e)
	}
	if _, e = s.Retry(context.Background(), "a", "p", old.ID); e != ErrRetryUnavailable {
		t.Fatalf("replaced retry %v", e)
	}
}

func waitForReplacement(t *testing.T, s *Service, id string) Run {
	t.Helper()
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		r, e := s.Latest(context.Background(), "a", "p")
		if e != nil {
			t.Fatal(e)
		}
		if r != nil && r.ID != id && r.Status != "running" {
			return *r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("replacement never finished")
	return Run{}
}

func TestReplacementLatestChoiceSurvivesBackendRestart(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	entered := make(chan struct{})
	gen := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		close(entered)
		<-ctx.Done()
		return Finding{}, ctx.Err()
	})
	s := New(db, retriever, gen, "embed-a")
	old, e := s.Start(context.Background(), "a", "p")
	if e != nil {
		t.Fatal(e)
	}
	waitSignal(t, entered)
	for _, threshold := range []float64{.85, .55, .65} {
		if _, e = s.Replace(context.Background(), "a", "p", old.ID, Options{Mode: "all", Threshold: threshold}, "wait"); e != nil {
			t.Fatal(e)
		}
	}
	s.Close()
	nextService := testService(t, db, retriever, generatorFunc(func(ctx context.Context, p Pair) (Finding, error) { return findingFor(p), nil }), "embed-a")
	next := waitForReplacement(t, nextService, old.ID)
	if next.Status != "completed" || next.SimilarityThreshold != .65 {
		t.Fatalf("last choice not restored: %+v", next)
	}
}

func TestCancelWaitingReplacementDoesNotStartSuccessor(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	entered := make(chan struct{})
	gen := generatorFunc(func(ctx context.Context, p Pair) (Finding, error) {
		close(entered)
		<-ctx.Done()
		return Finding{}, ctx.Err()
	})
	s := testService(t, db, retriever, gen, "embed-a")
	old, e := s.Start(context.Background(), "a", "p")
	if e != nil {
		t.Fatal(e)
	}
	waitSignal(t, entered)
	if _, e = s.Replace(context.Background(), "a", "p", old.ID, Options{Mode: "all", Threshold: .55}, "wait"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Cancel(context.Background(), "a", "p", old.ID); e != nil {
		t.Fatal(e)
	}
	var remaining int
	if e = db.QueryRow(`SELECT count(*) FROM analysis_replacements WHERE run_id=$1`, old.ID).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatalf("pending successor %d %v", remaining, e)
	}
	if _, e = s.Replace(context.Background(), "a", "p", old.ID, Options{Mode: "all", Threshold: .55}, "invalid"); e == nil {
		t.Fatal("invalid strategy accepted")
	}
}
