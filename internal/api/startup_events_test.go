package api

import (
	"context"
	"slices"
	"testing"
)

func TestStartupInvalidatesChangedProfileForExistingClients(t *testing.T) {
	dsn := testDatabase(t)
	s, err := NewWithEmbedder(dsn, map[string]string{"token-a": "a"}, &deterministicEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	cursor, err := readEventBatch(context.Background(), s.db, "a", eventCursor{})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Provider disabled by new configuration; the old SSE cursor is still valid.
	s, err = New(dsn, map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	batch, err := readEventBatch(context.Background(), s.db, "a", cursor.cursor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range batch.events {
		if slices.Contains(event.keys, "project:p") && slices.Contains(event.keys, "project:p:classification-profile") && slices.Contains(event.keys, "project:p:classifications") {
			found = true
		}
	}
	if !found {
		t.Fatalf("restart failed to invalidate project profile subscriptions: %+v", batch)
	}
	other, err := readEventBatch(context.Background(), s.db, "other", eventCursor{})
	if err != nil || len(other.events) != 0 {
		t.Fatalf("startup crossed workspace: %+v %v", other, err)
	}
}
