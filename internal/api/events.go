package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const analysisEventPrefix = "project:"
const analysisEventSuffix = ":analysis"

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
type analysisProgressSnapshot struct {
	ID                    string   `json:"id"`
	ProjectID             string   `json:"projectId"`
	Revision              string   `json:"revision"`
	CreatedAt             string   `json:"createdAt"`
	Status                string   `json:"status"`
	Stage                 string   `json:"stage"`
	Outcome               string   `json:"outcome"`
	ErrorCode             string   `json:"errorCode"`
	ReasonCode            string   `json:"reasonCode"`
	CompletedAt           string   `json:"completedAt"`
	Mode                  string   `json:"mode"`
	ActivePairID          string   `json:"activePairId"`
	SimilarityThreshold   float64  `json:"similarityThreshold"`
	ProcessedPairs        int      `json:"processedPairs"`
	DuplicatePairs        int      `json:"duplicatePairs"`
	ReusedPairs           int      `json:"reusedPairs"`
	NewPairs              int      `json:"newPairs"`
	CompletedPairs        int      `json:"completedPairs"`
	PendingPairs          int      `json:"pendingPairs"`
	RunningPairs          int      `json:"runningPairs"`
	FailedPairs           int      `json:"failedPairs"`
	CollisionPairs        int      `json:"collisionPairs"`
	CompatiblePairs       int      `json:"compatiblePairs"`
	InsufficientPairs     int      `json:"insufficientPairs"`
	TotalPairs            *int     `json:"totalPairs"`
	PossiblePairs         *int     `json:"possiblePairs"`
	NotSelectedPairs      *int     `json:"notSelectedPairs"`
	Stale                 bool     `json:"stale"`
	MissingRequirementIDs []string `json:"missingRequirementIds"`
}
type analysisProgressEvent struct {
	CursorID      string                   `json:"-"`
	CursorSeq     int64                    `json:"-"`
	SchemaVersion int                      `json:"schemaVersion"`
	ProjectID     string                   `json:"projectId"`
	RunID         string                   `json:"runId"`
	Revision      string                   `json:"revision"`
	Keys          []string                 `json:"keys,omitempty"`
	Snapshot      analysisProgressSnapshot `json:"snapshot"`
}
type eventBatch struct {
	cursor   eventCursor
	reset    bool
	events   []cacheEvent
	progress []analysisProgressEvent
}

func readAnalysisProgress(ctx context.Context, tx *sql.Tx, workspace, project string, profiles ...string) (*analysisProgressSnapshot, error) {
	var snapshot analysisProgressSnapshot
	profileCheck := len(profiles) >= 2
	generatorProfile, embeddingProfile := "", ""
	if profileCheck {
		generatorProfile, embeddingProfile = profiles[0], profiles[1]
	}
	var total, possible sql.NullInt64
	var missing []byte
	var stale bool
	var created time.Time
	var completed sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT r.id,r.project_id,r.revision::text,r.created_at,r.status,r.stage,r.outcome,r.error_code,r.reason_code,r.completed_at,r.mode,r.active_pair_id,r.similarity_threshold,r.processed_pairs,r.total_pairs,r.possible_pairs,r.missing_requirement_ids,
	count(*) FILTER(WHERE p.status='completed' AND p.verdict='duplicate'),count(*) FILTER(WHERE p.status='completed' AND p.reused_from<>''),count(*) FILTER(WHERE p.status='completed' AND p.reused_from=''),count(*) FILTER(WHERE p.status='completed'),count(*) FILTER(WHERE p.status='pending'),count(*) FILTER(WHERE p.status='running'),count(*) FILTER(WHERE p.status='failed'),
	count(*) FILTER(WHERE p.status='completed' AND p.verdict='collision'),count(*) FILTER(WHERE p.status='completed' AND p.verdict='compatible'),count(*) FILTER(WHERE p.status='completed' AND p.verdict='insufficient'),
	(r.source_signature <> (SELECT md5(coalesce(string_agg(md5(jsonb_build_array(q.id,q.version,q.text)::text),'' ORDER BY q.id),'')) FROM requirements q WHERE q.workspace=r.workspace AND q.project_id=r.project_id) OR ($3::boolean AND (r.generator_profile<>$4 OR (r.mode='related' AND r.embedding_profile<>$5))))
 FROM analysis_runs r LEFT JOIN analysis_pairs p ON p.run_id=r.id AND p.workspace=r.workspace AND p.project_id=r.project_id
 WHERE r.workspace=$1 AND r.project_id=$2 GROUP BY r.id ORDER BY r.seq DESC LIMIT 1`, workspace, project, profileCheck, generatorProfile, embeddingProfile).Scan(
		&snapshot.ID, &snapshot.ProjectID, &snapshot.Revision, &created, &snapshot.Status, &snapshot.Stage, &snapshot.Outcome, &snapshot.ErrorCode, &snapshot.ReasonCode, &completed, &snapshot.Mode, &snapshot.ActivePairID, &snapshot.SimilarityThreshold, &snapshot.ProcessedPairs, &total, &possible, &missing,
		&snapshot.DuplicatePairs, &snapshot.ReusedPairs, &snapshot.NewPairs, &snapshot.CompletedPairs, &snapshot.PendingPairs, &snapshot.RunningPairs, &snapshot.FailedPairs, &snapshot.CollisionPairs, &snapshot.CompatiblePairs, &snapshot.InsufficientPairs, &stale)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	snapshot.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	if completed.Valid {
		snapshot.CompletedAt = completed.Time.UTC().Format(time.RFC3339Nano)
	}
	if total.Valid {
		n := int(total.Int64)
		snapshot.TotalPairs = &n
	}
	if possible.Valid {
		n := int(possible.Int64)
		snapshot.PossiblePairs = &n
		if total.Valid {
			notSelected := n - int(total.Int64)
			snapshot.NotSelectedPairs = &notSelected
		}
	}
	if err = json.Unmarshal(missing, &snapshot.MissingRequirementIDs); err != nil {
		return nil, err
	}
	if snapshot.MissingRequirementIDs == nil {
		snapshot.MissingRequirementIDs = []string{}
	}
	snapshot.Stale = stale
	return &snapshot, nil
}

// Each poll is a coherent snapshot of retention, cursor validity and events.
// Cursor IDs and event payloads are always looked up within the authorized workspace.
func readEventBatch(ctx context.Context, db *sql.DB, workspace string, cursor eventCursor, profiles ...string) (eventBatch, error) {
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
		lastAnalysisEvent := map[string]cacheEvent{}
		for _, event := range result.events {
			for _, key := range event.keys {
				if !strings.HasPrefix(key, analysisEventPrefix) || !strings.HasSuffix(key, analysisEventSuffix) {
					continue
				}
				project := strings.TrimSuffix(strings.TrimPrefix(key, analysisEventPrefix), analysisEventSuffix)
				if project == "" {
					continue
				}
				lastAnalysisEvent[project] = event
			}
		}
		for project, cursorEvent := range lastAnalysisEvent {
			snapshot, err := readAnalysisProgress(ctx, tx, workspace, project, profiles...)
			if err != nil {
				return result, err
			}
			if snapshot == nil {
				continue
			}
			result.progress = append(result.progress, analysisProgressEvent{CursorID: cursorEvent.id, CursorSeq: cursorEvent.seq, SchemaVersion: 1, ProjectID: project, RunID: snapshot.ID, Revision: snapshot.Revision, Snapshot: *snapshot})
		}
		sort.Slice(result.progress, func(i, j int) bool { return result.progress[i].CursorSeq < result.progress[j].CursorSeq })
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

func writeAnalysisProgress(w http.ResponseWriter, event analysisProgressEvent) error {
	if strings.ContainsAny(event.CursorID, "\r\n") {
		return errors.New("invalid stored event ID")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %s\nevent: analysis.progress\ndata: %s\n\n", event.CursorID, payload)
	return err
}

func serveEvents(db *sql.DB, workspace string, w http.ResponseWriter, r *http.Request, profiles ...string) {
	serveEventsWithProfiles(db, workspace, w, r, 300*time.Millisecond, 15*time.Second, profiles...)
}

func serveEventsWithTiming(db *sql.DB, workspace string, w http.ResponseWriter, r *http.Request, pollInterval, heartbeatInterval time.Duration) {
	serveEventsWithProfiles(db, workspace, w, r, pollInterval, heartbeatInterval)
}

func serveEventsWithProfiles(db *sql.DB, workspace string, w http.ResponseWriter, r *http.Request, pollInterval, heartbeatInterval time.Duration, profiles ...string) {
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	id := r.Header.Get("Last-Event-ID")
	// Oversized or malformed cursors receive the same reset as unknown cursors.
	if len(id) > 128 || strings.ContainsAny(id, "\r\n") {
		id = ""
	}
	batch, err := readEventBatch(r.Context(), db, workspace, eventCursor{id: id}, profiles...)
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
		progressByID := make(map[string]*analysisProgressEvent, len(batch.progress))
		projectsWithProgress := make(map[string]bool, len(batch.progress))
		for i := range batch.progress {
			progressByID[batch.progress[i].CursorID] = &batch.progress[i]
			projectsWithProgress[batch.progress[i].ProjectID] = true
		}
		for _, event := range batch.events {
			keys := make([]string, 0, len(event.keys))
			for _, key := range event.keys {
				project := ""
				if strings.HasPrefix(key, analysisEventPrefix) && strings.HasSuffix(key, analysisEventSuffix) {
					project = strings.TrimSuffix(strings.TrimPrefix(key, analysisEventPrefix), analysisEventSuffix)
				}
				if project != "" && projectsWithProgress[project] {
					continue
				}
				keys = append(keys, key)
			}
			if progress := progressByID[event.id]; progress != nil {
				progress.Keys = keys
				if writeAnalysisProgress(w, *progress) != nil {
					return false
				}
			} else if len(keys) > 0 && writeCacheEvent(w, event.id, "cache.invalidate", keys) != nil {
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
			batch, err = readEventBatch(r.Context(), db, workspace, cursor, profiles...)
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
