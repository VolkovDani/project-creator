package api

import (
	"bufio"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func eventDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.Open(testDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func appendEvent(t *testing.T, db *sql.DB, id, workspace string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO cache_events(id,workspace,keys) VALUES($1,$2,ARRAY['project:p'])`, id, workspace); err != nil {
		t.Fatal(err)
	}
}
func TestEventReplayResetAndWorkspace(t *testing.T) {
	db := eventDB(t)
	appendEvent(t, db, "a1", "a")
	appendEvent(t, db, "b1", "b")
	appendEvent(t, db, "a2", "a")
	batch, err := readEventBatch(context.Background(), db, "a", eventCursor{id: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if batch.reset || len(batch.events) != 1 || batch.events[0].id != "a2" {
		t.Fatalf("replay: %+v", batch)
	}
	for _, id := range []string{"", "unknown", "b1"} {
		batch, err = readEventBatch(context.Background(), db, "a", eventCursor{id: id})
		if err != nil {
			t.Fatal(err)
		}
		if !batch.reset || batch.cursor.id != "a2" || len(batch.events) != 0 {
			t.Fatalf("reset %q: %+v", id, batch)
		}
	}
	if _, err = db.Exec(`DELETE FROM cache_events WHERE id='a1'`); err != nil {
		t.Fatal(err)
	}
	batch, err = readEventBatch(context.Background(), db, "a", eventCursor{id: "a1", seq: 1, initialized: true})
	if err != nil || !batch.reset {
		t.Fatalf("expired: %+v %v", batch, err)
	}
}

func TestEventResetSnapshotDoesNotLoseNewEvents(t *testing.T) {
	db := eventDB(t)
	appendEvent(t, db, "b1", "b")
	first, err := readEventBatch(context.Background(), db, "a", eventCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if !first.reset || first.cursor.id != "" {
		t.Fatalf("first: %+v", first)
	}
	appendEvent(t, db, "a1", "a")
	second, err := readEventBatch(context.Background(), db, "a", first.cursor)
	if err != nil {
		t.Fatal(err)
	}
	if second.reset || len(second.events) != 1 || second.events[0].id != "a1" {
		t.Fatalf("new: %+v", second)
	}
	if _, err = db.Exec(`DELETE FROM cache_events`); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, db, "b2", "b")
	third, err := readEventBatch(context.Background(), db, "a", second.cursor)
	if err != nil || !third.reset {
		t.Fatalf("gap: %+v %v", third, err)
	}
}

func TestEventStreamHeartbeatAndCancellation(t *testing.T) {
	db := eventDB(t)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		serveEventsWithTiming(db, "a", w, r, 5*time.Millisecond, 15*time.Millisecond)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal(response.Header)
	}
	blocks := make(chan string, 10)
	go func() {
		defer close(blocks)
		scanner := bufio.NewScanner(response.Body)
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				blocks <- strings.Join(lines, "\n")
				lines = nil
			} else {
				lines = append(lines, line)
			}
		}
	}()
	next := func() string {
		t.Helper()
		select {
		case b := <-blocks:
			return b
		case <-time.After(2 * time.Second):
			t.Fatal("stream timed out")
			return ""
		}
	}
	if b := next(); !strings.Contains(b, "retry: 3000") || !strings.Contains(b, "event: cache.reset") {
		t.Fatal(b)
	}
	if b := next(); !strings.Contains(b, ": heartbeat") {
		t.Fatal(b)
	}
	cancel()
	response.Body.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
}

func TestEventEncodingAndCursorClearing(t *testing.T) {
	w := httptest.NewRecorder()
	if err := writeCacheEvent(w, "", "cache.reset", nil); err != nil {
		t.Fatal(err)
	}
	if got := w.Body.String(); got != "id: \nevent: cache.reset\ndata: {\"schemaVersion\":1,\"keys\":[]}\n\n" {
		t.Fatal(got)
	}
	w = httptest.NewRecorder()
	if err := writeCacheEvent(w, "safe", "cache.invalidate", []string{"project:p\nquoted\""}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(w.Body.String(), "\ndata:") != 1 || strings.Contains(w.Body.String(), "\nquoted") {
		t.Fatal(w.Body.String())
	}
	if err := writeCacheEvent(w, "bad\nevent: injected", "cache.reset", nil); err == nil {
		t.Fatal("accepted event ID newline")
	}
}

func TestEventsVisibleOnlyAfterCommit(t *testing.T) {
	db := eventDB(t)
	first, err := readEventBatch(context.Background(), db, "a", eventCursor{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO cache_events(id,workspace,keys) VALUES('pending','a',ARRAY['project:p'])`); err != nil {
		t.Fatal(err)
	}
	during, err := readEventBatch(context.Background(), db, "a", first.cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(during.events) != 0 {
		t.Fatal("uncommitted event visible")
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	after, err := readEventBatch(context.Background(), db, "a", during.cursor)
	if err != nil {
		t.Fatal(err)
	}
	if after.reset || len(after.events) != 1 || after.events[0].id != "pending" {
		t.Fatalf("commit: %+v", after)
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO cache_events(id,workspace,keys) VALUES('rolled-back','a',ARRAY['project:p'])`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := readEventBatch(context.Background(), db, "a", after.cursor)
	if err != nil || len(rolledBack.events) != 0 {
		t.Fatalf("rollback: %+v %v", rolledBack, err)
	}
}
