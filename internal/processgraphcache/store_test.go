package processgraphcache

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/processgraph"
	"github.com/VolkovDani/project-creator/internal/storage"
)

func TestGraphCachePersistenceAndReuse(t *testing.T) {
	ctx := t.Context()
	dsn, db := testDatabase(t)
	store := New(db, "profile-1")
	first := cachedRun("a", "p", "topic-1", "snapshot-1", "run-1", 0)
	second := cachedRun("a", "p", "topic-2", "snapshot-2", "run-2", 0)
	failed := cachedRun("a", "p", "topic-1", "snapshot-1", "failed", 1)
	other := cachedRun("b", "q", "topic-1", "snapshot-1", "other-project", 0)
	for _, run := range []processgraph.CachedRun{first, second, failed, other} {
		if err := store.Save(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	store = New(reopened, "profile-1")
	expectLatest(t, store, failed)
	expectLatest(t, store, other)

	expectReuseMiss(t, store, first.Target, "missing")
	expectReuseMiss(t, store, processgraph.Target{Workspace: "b", Project: "p", Keyword: first.Target.Keyword}, first.SnapshotKey)
	expectReuseMiss(t, store, second.Target, first.SnapshotKey)
	current := New(reopened, "profile-2")
	expectReuseMiss(t, current, first.Target, first.SnapshotKey)
	latest, err := current.Latest(ctx, "a", "p")
	if err != nil || latest.Result.Status != "" {
		t.Fatalf("new profile latest = %+v, %v", latest, err)
	}
	expectCount(t, reopened, "p", 3)

	got, err := store.Reuse(ctx, first.Target, first.SnapshotKey, "promoted")
	first.Result.Run.ID = "promoted"
	if err != nil || !reflect.DeepEqual(got, first.Result) {
		t.Fatalf("reused result = %+v, %v, want %+v", got, err, first.Result)
	}
	expectLatest(t, store, first)
	expectCount(t, reopened, "p", 3)

	failedOnly := cachedRun("a", "p", "topic-3", "snapshot-3", "failed-only", 1)
	failedOnly.Target.TermID, failedOnly.Target.Keyword = "term-3", ""
	if err := store.Save(ctx, failedOnly); err != nil {
		t.Fatal(err)
	}
	expectLatest(t, store, failedOnly)
	expectReuseMiss(t, store, failedOnly.Target, failedOnly.SnapshotKey)

	first.Result.Run.ID = "regenerated"
	if err := store.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	expectLatest(t, store, first)
	expectCount(t, reopened, "p", 4)
	if _, err := reopened.ExecContext(ctx, `UPDATE process_graph_cache SET format_version=$1 WHERE snapshot_key='snapshot-1' AND project_id='p'`, formatVersion+1); err != nil {
		t.Fatal(err)
	}
	expectReuseMiss(t, store, first.Target, first.SnapshotKey)
	expectLatest(t, store, failedOnly)
	expectCount(t, reopened, "p", 4)
}

func TestGraphCacheTTL(t *testing.T) {
	ctx := t.Context()
	_, db := testDatabase(t)
	store := NewWithTTL(db, "profile-1", time.Hour)
	forever := New(db, "profile-1")
	fresh := cachedRun("a", "p", "topic-1", "snapshot-1", "run-1", 0)
	expired := cachedRun("a", "p", "topic-2", "snapshot-2", "run-2", 0)
	for _, run := range []processgraph.CachedRun{fresh, expired} {
		if err := store.Save(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE process_graph_cache SET created_at=statement_timestamp()-interval '2 hours' WHERE snapshot_key=$1`, expired.SnapshotKey); err != nil {
		t.Fatal(err)
	}
	expectLatest(t, store, fresh)
	expectLatest(t, forever, expired)
	expectReuseMiss(t, store, expired.Target, expired.SnapshotKey)
	expectCount(t, db, "p", 2)

	var before, after time.Time
	createdAt := `SELECT created_at FROM process_graph_cache WHERE snapshot_key=$1`
	if err := db.QueryRowContext(ctx, createdAt, fresh.SnapshotKey).Scan(&before); err != nil {
		t.Fatal(err)
	}
	got, err := store.Reuse(ctx, fresh.Target, fresh.SnapshotKey, "promoted")
	fresh.Result.Run.ID = "promoted"
	if err != nil || !reflect.DeepEqual(got, fresh.Result) {
		t.Fatalf("fresh reuse = %+v, %v, want %+v", got, err, fresh.Result)
	}
	if err := db.QueryRowContext(ctx, createdAt, fresh.SnapshotKey).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatalf("reuse changed cache age from %s to %s", before, after)
	}
	if _, err := db.ExecContext(ctx, `UPDATE process_graph_cache SET created_at=statement_timestamp()-interval '2 hours'`); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, fresh.Target.Workspace, fresh.Target.Project)
	if err != nil || latest.Result.Status != "" {
		t.Fatalf("latest after expiry = %+v, %v", latest, err)
	}
	expectLatest(t, forever, fresh)
	got, err = forever.Reuse(ctx, fresh.Target, fresh.SnapshotKey, "indefinite")
	if err != nil || got.Run == nil || got.Run.ID != "indefinite" {
		t.Fatalf("indefinite reuse = %+v, %v", got, err)
	}
	if err := store.Save(ctx, expired); err != nil {
		t.Fatal(err)
	}
	expectLatest(t, store, expired)
	expectCount(t, db, "p", 2)
}

func TestGraphCacheClear(t *testing.T) {
	ctx := t.Context()
	_, db := testDatabase(t)
	first := cachedRun("a", "p", "topic-1", "snapshot-1", "run-1", 0)
	second := cachedRun("b", "q", "topic-2", "snapshot-2", "run-2", 0)
	store := New(db, "profile-1")
	for _, profile := range []string{"profile-1", "profile-2"} {
		if err := New(db, profile).Save(ctx, first); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(ctx, second); err != nil {
		t.Fatal(err)
	}
	expectCount(t, db, "missing", 0)
	expectCount(t, db, "p", 2)
	expectCount(t, db, "", 3)
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE cache_events ADD CONSTRAINT reject_clear
		 CHECK (keys <> ARRAY['project:p:process-graph']) NOT VALID
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := Clear(ctx, db, "p"); err == nil {
		t.Fatal("clear committed despite a rejected invalidation")
	}
	expectCount(t, db, "p", 2)
	if _, err := db.ExecContext(ctx, `ALTER TABLE cache_events DROP CONSTRAINT reject_clear`); err != nil {
		t.Fatal(err)
	}
	if n, err := Clear(ctx, db, "p"); err != nil || n != 2 {
		t.Fatalf("scoped clear = %d, %v", n, err)
	}
	expectReuseMiss(t, store, first.Target, first.SnapshotKey)
	latest, err := store.Latest(ctx, "a", "p")
	if err != nil || latest.Result.Status != "" {
		t.Fatalf("cleared latest = %+v, %v", latest, err)
	}
	expectLatest(t, store, second)
	expectCount(t, db, "", 1)
	expectEventCount(t, db, "a", "project:p:process-graph", 1)
	expectEventCount(t, db, "b", "project:p:process-graph", 0)
	if n, err := Clear(ctx, db, "p"); err != nil || n != 0 {
		t.Fatalf("empty scoped clear = %d, %v", n, err)
	}
	expectEventCount(t, db, "a", "project:p:process-graph", 1)
	// New completions can populate the cache after a clear.
	if err := store.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO cache_events(id,workspace,keys)
		SELECT 'cursor-'||n,'a',ARRAY['projects'] FROM generate_series(1,1005) n
	`); err != nil {
		t.Fatal(err)
	}
	if n, err := Clear(ctx, db, ""); err != nil || n != 2 {
		t.Fatalf("installation-wide clear = %d, %v", n, err)
	}
	expectCount(t, db, "", 0)
	expectEventCount(t, db, "a", "project:p:process-graph", 1)
	expectEventCount(t, db, "b", "project:q:process-graph", 1)
	var eventCount int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM cache_events`).Scan(&eventCount)
	if err != nil || eventCount != 1000 {
		t.Fatalf("outbox retention = %d, %v", eventCount, err)
	}
	var unrelated int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&unrelated)
	if err != nil || unrelated != 2 {
		t.Fatalf("projects after clear = %d, %v", unrelated, err)
	}
}

func TestGraphCacheReuseWaitsForConcurrentClear(t *testing.T) {
	_, db := testDatabase(t)
	run := cachedRun("a", "p", "topic-1", "snapshot-1", "run-1", 0)
	store := New(db, "profile-1")
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	clear, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear.Rollback()
	var clearPID int
	if err := clear.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&clearPID); err != nil {
		t.Fatal(err)
	}
	if _, err := clear.ExecContext(ctx, `DELETE FROM process_graph_cache WHERE project_id='p'`); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result processgraph.RunResult
		err    error
	}
	reuseDone := make(chan outcome, 1)
	go func() {
		reused, err := store.Reuse(ctx, run.Target, run.SnapshotKey, "promoted")
		reuseDone <- outcome{reused, err}
	}()
	// Wait until the reuse statement is blocked by the still-uncommitted
	// clear, so this also exercises the row lock and PostgreSQL recheck.
	for {
		var blocked bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS(
			 SELECT 1 FROM pg_stat_activity
			 WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid))
			)
		`, clearPID).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case got := <-reuseDone:
			t.Fatalf("reuse finished before the clear committed: %+v, %v", got.result, got.err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := clear.Commit(); err != nil {
		t.Fatal(err)
	}
	got := <-reuseDone
	if got.err != nil || got.result.Status != "" {
		t.Fatalf("reuse after clear = %+v, %v", got.result, got.err)
	}
	expectCount(t, db, "p", 0)
}

func cachedRun(workspace, project, keyword, snapshot, id string, failed int) processgraph.CachedRun {
	return processgraph.CachedRun{
		Target:      processgraph.Target{Workspace: workspace, Project: project, Keyword: keyword},
		SnapshotKey: snapshot,
		Result: processgraph.RunResult{
			Status: "suggested",
			Run:    &processgraph.Progress{ID: id, Topic: keyword, Revision: 3, Completed: 2, Total: 2, Failed: failed},
			Draft: &processgraph.Draft{
				Nodes: []processgraph.Node{
					{ID: "user", Kind: "actor", Label: "User"},
					{ID: "submit", Kind: "action", Label: "Submit a request"},
				},
				Edges: []processgraph.Edge{{
					From: "user", To: "submit", Kind: "performs",
					Evidence: []processgraph.Evidence{{RequirementID: "r", Version: 4, Quote: "The user submits a request.", Start: 7, End: 34}},
				}},
				SourceCount: 1, Partial: true,
			},
		},
	}
}

func expectLatest(t *testing.T, store *Store, want processgraph.CachedRun) {
	t.Helper()
	got, err := store.Latest(t.Context(), want.Target.Workspace, want.Target.Project)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("latest result = %+v, %v, want %+v", got, err, want)
	}
}

func expectReuseMiss(t *testing.T, store *Store, target processgraph.Target, snapshot string) {
	t.Helper()
	got, err := store.Reuse(t.Context(), target, snapshot, "unexpected-reuse")
	if err != nil || got.Status != "" || got.Run != nil || got.Draft != nil {
		t.Fatalf("expected cache miss, got %+v, %v", got, err)
	}
}

func expectCount(t *testing.T, db *sql.DB, project string, want int64) {
	t.Helper()
	got, err := Count(t.Context(), db, project)
	if err != nil || got != want {
		t.Fatalf("count for project %q = %d, %v, want %d", project, got, err, want)
	}
}

func expectEventCount(t *testing.T, db *sql.DB, workspace, key string, want int) {
	t.Helper()
	var got int
	err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM cache_events WHERE workspace=$1 AND keys=ARRAY[$2]`, workspace, key).Scan(&got)
	if err != nil || got != want {
		t.Fatalf("event count for %q in %q = %d, %v, want %d", key, workspace, got, err, want)
	}
}

func testDatabase(t *testing.T) (string, *sql.DB) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required in CI")
		}
		t.Skip("PostgreSQL integration: set TEST_DATABASE_URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(nonce[:])
	if _, err = admin.ExecContext(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(t.Context(), `INSERT INTO projects VALUES('p','a','P','G','','now','now'), ('q','b','Q','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	return u.String(), db
}
