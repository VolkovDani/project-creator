package rag

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type recoveryEmbedder struct {
	profile string
	calls   atomic.Int32
	active  atomic.Int32
	maximum atomic.Int32
	fail    atomic.Bool
	started chan struct{}
	release chan struct{}
}

func (e *recoveryEmbedder) Profile() string { return e.profile }
func (e *recoveryEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls.Add(1)
	active := e.active.Add(1)
	for maximum := e.maximum.Load(); active > maximum && !e.maximum.CompareAndSwap(maximum, active); maximum = e.maximum.Load() {
	}
	defer e.active.Add(-1)
	if e.started != nil {
		select {
		case e.started <- struct{}{}:
		default:
		}
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if e.fail.Load() {
		return nil, errors.New("temporary embedding failure")
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 1024)
		out[i][0] = 1
	}
	return out, nil
}

func waitQueueState(t *testing.T, dbQuery func() (string, int, time.Time), wantStatus string, wantAttempts int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, attempts, _ := dbQuery()
		if status == wantStatus && attempts == wantAttempts {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue state did not reach %s/%d (last %s/%d)", wantStatus, wantAttempts, status, attempts)
		}
		<-ticker.C
	}
}

func TestQueueMultipleWorkersReindexExactlyOnceAndSurviveRestart(t *testing.T) {
	db := ragDB(t)
	if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a'`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		if _, err = tx.Exec(`INSERT INTO requirements VALUES($1,'a','p',$2,1,'now','now')`, fmt.Sprintf("bulk-%03d", i), fmt.Sprintf("Requirement %d", i)); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	embedder := &recoveryEmbedder{profile: "model:queue-recovery"}
	first, second := New(db, embedder), New(db, embedder)
	defer first.Close()
	defer second.Close()
	if err = first.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if err = second.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			progress, summaryErr := first.Summary(context.Background(), "a", "p")
			if summaryErr != nil {
				t.Fatal(summaryErr)
			}
			if progress.Total == 200 && progress.Ready == 200 && progress.Queued == 0 && progress.Running == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("queue did not finish all rows: %+v", progress)
			}
			<-ticker.C
		}
	}
	wait()
	if got := embedder.calls.Load(); got != 200 {
		t.Fatalf("embedding calls=%d, want one per committed requirement", got)
	}
	if got := embedder.maximum.Load(); got > 1 {
		t.Fatalf("concurrent embedding calls=%d, want at most one", got)
	}
	var ready, chunks int
	if err = db.QueryRow(`SELECT count(*) FROM requirement_index WHERE workspace='a' AND project_id='p' AND status='ready'`).Scan(&ready); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM requirement_chunks WHERE workspace='a' AND project_id='p'`).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if ready != 200 || chunks != 200 {
		t.Fatalf("persisted results ready=%d chunks=%d; want 200 each", ready, chunks)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := New(db, embedder)
	defer restarted.Close()
	if err = restarted.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	var outstanding int
	if err = db.QueryRow(`SELECT count(*) FROM requirement_index WHERE workspace='a' AND project_id='p' AND status IN ('queued','running')`).Scan(&outstanding); err != nil {
		t.Fatal(err)
	}
	if outstanding != 0 {
		t.Fatalf("restart requeued ready requirements: outstanding=%d", outstanding)
	}
	if err = restarted.Close(); err != nil {
		t.Fatal(err)
	}
	if got := embedder.calls.Load(); got != 200 {
		t.Fatalf("restart re-embedded ready rows: calls=%d", got)
	}
}

func TestQueueRetriesTransientFailureThreeTimesThenAllowsManualRetry(t *testing.T) {
	db := ragDB(t)
	if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND id<>'r'`); err != nil {
		t.Fatal(err)
	}
	embedder := &recoveryEmbedder{profile: "model:retry"}
	embedder.fail.Store(true)
	s := New(db, embedder)
	defer s.Close()
	if err := s.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	rowState := func() (string, int, time.Time) {
		t.Helper()
		var status string
		var attempts int
		var available time.Time
		if err := db.QueryRow(`SELECT status,attempts,available_at FROM requirement_index WHERE workspace='a' AND project_id='p' AND requirement_id='r' AND profile=$1`, embedder.profile).Scan(&status, &attempts, &available); err != nil {
			t.Fatal(err)
		}
		return status, attempts, available
	}
	for attempt := 1; attempt <= 3; attempt++ {
		waitQueueState(t, rowState, "failed", attempt)
		if got := embedder.calls.Load(); got != int32(attempt) {
			t.Fatalf("attempt %d: provider calls=%d", attempt, got)
		}
		if attempt < 3 {
			if _, err := db.Exec(`UPDATE requirement_index SET available_at=clock_timestamp()-interval '1 second' WHERE workspace='a' AND project_id='p' AND requirement_id='r' AND profile=$1`, embedder.profile); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Give the worker several polling opportunities and ensure exhaustion does not
	// trigger another model call while the failed item remains in the queue.
	deadline := time.Now().Add(700 * time.Millisecond)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		if got := embedder.calls.Load(); got != 3 {
			t.Fatalf("exhausted job was retried automatically: calls=%d", got)
		}
		<-ticker.C
	}
	embedder.fail.Store(false)
	state, err := s.Index(context.Background(), "a", "p", "r")
	if err != nil || state.Status != "ready" {
		t.Fatalf("manual retry after recovery: %+v %v", state, err)
	}
	if got := embedder.calls.Load(); got != 4 {
		t.Fatalf("manual retry calls=%d, want 4 total", got)
	}
}

func TestQueueRecoversExpiredLeaseButDoesNotRetryExhaustedLease(t *testing.T) {
	t.Run("recover in-flight attempt", func(t *testing.T) {
		db := ragDB(t)
		if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND id<>'r'`); err != nil {
			t.Fatal(err)
		}
		embedder := &recoveryEmbedder{profile: "model:lease-recovery"}
		if _, err := db.Exec(`INSERT INTO embedding_profiles(workspace,profile) VALUES('a',$1)`, embedder.profile); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until,error_code,attempts,available_at)
			VALUES('a','p','r',$1,1,'running','abandoned-attempt',clock_timestamp()-interval '1 second','',1,clock_timestamp())`, embedder.profile); err != nil {
			t.Fatal(err)
		}
		s := New(db, embedder)
		defer s.Close()
		if err := s.StartQueue([]string{"a"}); err != nil {
			t.Fatal(err)
		}
		rowState := func() (string, int, time.Time) {
			t.Helper()
			var status string
			var attempts int
			var lease time.Time
			if err := db.QueryRow(`SELECT status,attempts,lease_until FROM requirement_index WHERE workspace='a' AND project_id='p' AND requirement_id='r' AND profile=$1`, embedder.profile).Scan(&status, &attempts, &lease); err != nil {
				t.Fatal(err)
			}
			return status, attempts, lease
		}
		waitQueueState(t, rowState, "ready", 2)
		if embedder.calls.Load() != 1 {
			t.Fatalf("expired lease recovery calls=%d, want 1", embedder.calls.Load())
		}
	})

	t.Run("exhausted expired attempt", func(t *testing.T) {
		db := ragDB(t)
		if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND id<>'r'`); err != nil {
			t.Fatal(err)
		}
		embedder := &recoveryEmbedder{profile: "model:lease-exhausted"}
		if _, err := db.Exec(`INSERT INTO embedding_profiles(workspace,profile) VALUES('a',$1)`, embedder.profile); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until,error_code,attempts,available_at)
			VALUES('a','p','r',$1,1,'running','abandoned-attempt',clock_timestamp()-interval '1 second','',3,clock_timestamp())`, embedder.profile); err != nil {
			t.Fatal(err)
		}
		s := New(db, embedder)
		defer s.Close()
		if err := s.StartQueue([]string{"a"}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			var status string
			var attempts int
			if err := db.QueryRow(`SELECT status,attempts FROM requirement_index WHERE workspace='a' AND project_id='p' AND requirement_id='r' AND profile=$1`, embedder.profile).Scan(&status, &attempts); err != nil {
				t.Fatal(err)
			}
			if status == "failed" && attempts == 3 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("exhausted lease was not marked failed: %s/%d", status, attempts)
			}
			<-ticker.C
		}
		if got := embedder.calls.Load(); got != 0 {
			t.Fatalf("exhausted lease called provider %d times", got)
		}
	})
}

func TestQueueLateResultCannotOverwriteNewLeaseOwner(t *testing.T) {
	db := ragDB(t)
	if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND id<>'r'`); err != nil {
		t.Fatal(err)
	}
	embedder := &recoveryEmbedder{profile: "model:lease-fence", started: make(chan struct{}, 1), release: make(chan struct{})}
	s := New(db, embedder)
	defer s.Close()
	if err := s.StartQueue([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-embedder.started:
	case <-time.After(10 * time.Second):
		t.Fatal("queue did not begin embedding")
	}
	if _, err := db.Exec(`UPDATE requirement_index SET attempt='new-owner',lease_until=clock_timestamp()+interval '2 minutes' WHERE workspace='a' AND project_id='p' AND requirement_id='r' AND profile=$1`, embedder.profile); err != nil {
		t.Fatal(err)
	}
	close(embedder.release)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var status, attempt string
	if err := db.QueryRow(`SELECT status,attempt FROM requirement_index WHERE workspace='a' AND project_id='p' AND requirement_id='r' AND profile=$1`, embedder.profile).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "running" || attempt != "new-owner" {
		t.Fatalf("late result overwrote current owner: status=%s attempt=%s", status, attempt)
	}
	var chunks int
	if err := db.QueryRow(`SELECT count(*) FROM requirement_chunks WHERE workspace='a' AND project_id='p' AND requirement_id='r'`).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if chunks != 0 {
		t.Fatalf("late owner published %d chunks", chunks)
	}
}
