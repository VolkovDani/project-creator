package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type eventCursor struct {
	seq         int64
	id          string
	initialized bool
}
type cacheEvent struct {
	seq  int64
	id   string
	keys []string
}
type eventBatch struct {
	cursor eventCursor
	reset  bool
	events []cacheEvent
}

// Each poll is a coherent snapshot of retention, cursor validity and events.
// Cursor IDs and event payloads are always looked up within the authorized workspace.
func readEventBatch(ctx context.Context, db *sql.DB, workspace string, cursor eventCursor) (eventBatch, error) {
	result := eventBatch{cursor: cursor}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var minimum, maximum int64
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(min(seq),0),coalesce(max(seq),0) FROM cache_events`).Scan(&minimum, &maximum); err != nil {
		return result, err
	}
	reset := !cursor.initialized && cursor.id == ""
	if cursor.id != "" {
		var seq int64
		err = tx.QueryRowContext(ctx, `SELECT seq FROM cache_events WHERE id=$1 AND workspace=$2`, cursor.id, workspace).Scan(&seq)
		if err == sql.ErrNoRows {
			reset = true
		} else if err != nil {
			return result, err
		} else if !cursor.initialized {
			result.cursor.seq = seq
		}
	} else if cursor.initialized && minimum > cursor.seq+1 {
		reset = true
	}
	if reset {
		var id string
		err = tx.QueryRowContext(ctx, `SELECT id FROM cache_events WHERE workspace=$1 ORDER BY seq DESC LIMIT 1`, workspace).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			return result, err
		}
		result.cursor = eventCursor{seq: maximum, id: id, initialized: true}
		result.reset = true
	} else {
		rows, err := tx.QueryContext(ctx, `SELECT seq,id,array_to_json(keys)::text FROM cache_events WHERE workspace=$1 AND seq>$2 ORDER BY seq LIMIT 256`, workspace, result.cursor.seq)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var event cacheEvent
			var keys string
			if err = rows.Scan(&event.seq, &event.id, &keys); err != nil {
				rows.Close()
				return result, err
			}
			if err = json.Unmarshal([]byte(keys), &event.keys); err != nil {
				rows.Close()
				return result, err
			}
			if event.keys == nil {
				event.keys = []string{}
			}
			result.events = append(result.events, event)
			result.cursor = eventCursor{seq: event.seq, id: event.id, initialized: true}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		result.cursor.initialized = true
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func writeCacheEvent(w http.ResponseWriter, id, name string, keys []string) error {
	if strings.ContainsAny(id, "\r\n") {
		return errors.New("invalid stored event ID")
	}
	if keys == nil {
		keys = []string{}
	}
	payload, err := json.Marshal(struct {
		SchemaVersion int      `json:"schemaVersion"`
		Keys          []string `json:"keys"`
	}{1, keys})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", id, name, payload)
	return err
}

func serveEvents(db *sql.DB, workspace string, w http.ResponseWriter, r *http.Request) {
	serveEventsWithTiming(db, workspace, w, r, 300*time.Millisecond, 15*time.Second)
}

func serveEventsWithTiming(db *sql.DB, workspace string, w http.ResponseWriter, r *http.Request, pollInterval, heartbeatInterval time.Duration) {
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	id := r.Header.Get("Last-Event-ID")
	// Oversized or malformed cursors receive the same reset as unknown cursors.
	if len(id) > 128 || strings.ContainsAny(id, "\r\n") {
		id = ""
	}
	batch, err := readEventBatch(r.Context(), db, workspace, eventCursor{id: id})
	if err != nil {
		http.Error(w, "event stream unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	flush := func() bool {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return controller.Flush() == nil
	}
	if _, err = fmt.Fprint(w, "retry: 3000\n"); err != nil {
		return
	}
	send := func(batch eventBatch) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if batch.reset {
			if writeCacheEvent(w, batch.cursor.id, "cache.reset", nil) != nil {
				return false
			}
		}
		for _, event := range batch.events {
			if writeCacheEvent(w, event.id, "cache.invalidate", event.keys) != nil {
				return false
			}
		}
		return flush()
	}
	if !batch.reset && len(batch.events) == 0 {
		if _, err = fmt.Fprint(w, "\n"); err != nil {
			return
		}
	}
	if !send(batch) {
		return
	}
	cursor := batch.cursor
	poll := time.NewTicker(pollInterval)
	defer poll.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			batch, err = readEventBatch(r.Context(), db, workspace, cursor)
			if err != nil {
				return
			} // Close and let EventSource reconnect; never disclose SQL details.
			if batch.reset || len(batch.events) > 0 {
				if !send(batch) {
					return
				}
			}
			cursor = batch.cursor
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprint(w, ": heartbeat\n\n"); err != nil || !flush() {
				return
			}
		}
	}
}
