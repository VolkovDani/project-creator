package rag

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fenceEmbedder struct{ calls atomic.Int32 }

func (p *fenceEmbedder) Profile() string { return "fence-v1" }
func (p *fenceEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	p.calls.Add(1)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, Dimensions)
		out[i][0] = 1
	}
	return out, nil
}

func prepareFenceQueue(t *testing.T, db *sql.DB, profile string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO embedding_profiles(workspace,profile) VALUES('a',$1);
`, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until)
 VALUES('a','p','r',$1,1,'queued','',clock_timestamp())`, profile); err != nil {
		t.Fatal(err)
	}
}

func TestQueueRechecksSourceBeforeEmbedding(t *testing.T) {
	db := ragDB(t)
	fake := &fenceEmbedder{}
	s := New(db, fake)
	prepareFenceQueue(t, db, fake.Profile())
	item, err := s.claimQueue(context.Background(), "a", fake.Profile())
	if err != nil || item == nil {
		t.Fatalf("claim: %v %v", item, err)
	}
	if _, err := db.Exec(`UPDATE requirements SET text='current source',version=2 WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	s.processQueueItem(context.Background(), *item)
	if fake.calls.Load() != 0 {
		t.Fatal("stale source was passed to embedder")
	}
}

func TestQueueSummaryCountsLegacyRunningAttemptOnce(t *testing.T) {
	db := ragDB(t)
	fake := &fenceEmbedder{}
	s := New(db, fake)
	prepareFenceQueue(t, db, fake.Profile())
	if _, err := db.Exec(`UPDATE requirement_index SET status='pending',attempt='legacy',lease_until=clock_timestamp()+interval '2 minutes' WHERE requirement_id='r'`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Summary(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Running != 1 || got.Queued != 0 {
		t.Fatalf("double-counted legacy attempt: %+v", got)
	}
}

func TestQueueProfileChangeAndDisableFenceInFlightResult(t *testing.T) {
	db := ragDB(t)
	if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND id<>'r'`); err != nil {
		t.Fatal(err)
	}
	old := &recoveryEmbedder{profile: "profile-old", started: make(chan struct{}, 1), release: make(chan struct{})}
	first := New(db, old)
	t.Cleanup(func() { _ = first.Close() })
	if err := first.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.started:
	case <-time.After(5 * time.Second):
		t.Fatal("inference did not start")
	}
	next := New(db, &fenceEmbedder{})
	t.Cleanup(func() { _ = next.Close() })
	if err := next.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	close(old.release)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	waitFenceReady(t, next)
	var stale int
	if err := db.QueryRow(`SELECT count(*) FROM requirement_chunks WHERE profile=$1`, old.profile).Scan(&stale); err != nil || stale != 0 {
		t.Fatalf("stale vectors: %d %v", stale, err)
	}
	disabled := New(db, nil)
	if err := disabled.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Index(context.Background(), "a", "p", "r"); !errors.Is(err, ErrInactiveProfile) {
		t.Fatalf("inactive provider accepted: %v", err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	waitFenceReady(t, first)
	if old.calls.Load() != 2 {
		t.Fatalf("reactivation did not requeue interrupted profile: calls=%d", old.calls.Load())
	}
}

func waitFenceReady(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		progress, err := s.Summary(context.Background(), "a", "p")
		if err != nil {
			t.Fatal(err)
		}
		if progress.Ready == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("profile did not become ready")
}

func TestEmbeddingSessionLossCancelsCall(t *testing.T) {
	db := ragDB(t)
	s := New(db, &fenceEmbedder{})
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.withEmbeddingLock(context.Background(), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("lock not acquired")
	}
	if _, err := db.Exec(`SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype='advisory' AND objid=724183094 AND granted AND pid<>pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost session: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lost session did not cancel embedding")
	}
}

func TestManualIndexWaitsForLeaseOfLostEmbeddingSession(t *testing.T) {
	db := ragDB(t)
	fake := &fenceEmbedder{}
	s := New(db, fake)
	if _, err := db.Exec(`INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until,attempts)
 VALUES('a','other','r2',$1,1,'running','still-cancelling',clock_timestamp()+interval '2 minutes',1)`, fake.Profile()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := s.Index(ctx, "a", "p", "r")
	if !errors.Is(err, context.DeadlineExceeded) || fake.calls.Load() != 0 {
		t.Fatalf("overlapped live lease: calls=%d err=%v", fake.calls.Load(), err)
	}
}
