package rag

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func ragDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required")
		}
		t.Skip("PostgreSQL integration requires TEST_DATABASE_URL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var bytes [12]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		t.Fatal(err)
	}
	schema := "rag_" + hex.EncodeToString(bytes[:])
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`INSERT INTO projects VALUES('p','a','Project','Goal','','now','now'),('other','a','Other','Goal','','now','now'),('secret','b','Secret','Goal','','now','now'); INSERT INTO requirements VALUES('r','a','p','Требование: оплата картой.',1,'now','now'),('r2','a','other','Other project',1,'now','now'),('hidden','b','secret','Secret requirement',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestIndexLifecycleProfileAndIsolation(t *testing.T) {
	ctx := context.Background()
	db := ragDB(t)
	embedder := &fakeEmbedder{profile: "model:a"}
	s := New(db, embedder)
	status, err := s.Status(ctx, "a", "p", 20, 0)
	if err != nil || len(status) != 1 || status[0].Status != "pending" {
		t.Fatalf("status: %+v %v", status, err)
	}
	for _, operation := range []func() error{
		func() error { _, err := s.Index(ctx, "b", "p", "r"); return err },
		func() error { _, err := s.Index(ctx, "a", "p", "hidden"); return err },
		func() error { _, err := s.Search(ctx, "b", "p", "search", 5); return err },
	} {
		if err := operation(); !errors.Is(err, ErrNotFound) {
			t.Fatalf("scope: %v", err)
		}
	}
	if embedder.calls != 0 {
		t.Fatal("unauthorized provider call")
	}
	first, err := s.Index(ctx, "a", "p", "r")
	if err != nil || first.Status != "ready" || first.Version != 1 {
		t.Fatalf("index: %+v %v", first, err)
	}
	if _, err = s.Index(ctx, "a", "p", "r"); err != nil || embedder.calls != 1 {
		t.Fatalf("not idempotent: %d %v", embedder.calls, err)
	}
	if _, err = s.Index(ctx, "a", "other", "r2"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Index(ctx, "b", "secret", "hidden"); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(ctx, "a", "p", "оплата", 20)
	if err != nil || len(hits) != 1 || hits[0].RequirementID != "r" || hits[0].SourceText != "Требование: оплата картой." || hits[0].Score < 0.99 {
		t.Fatalf("search: %+v %v", hits, err)
	}
	changed := New(db, &fakeEmbedder{profile: "model:b"})
	hits, err = changed.Search(ctx, "a", "p", "оплата", 20)
	if err != nil || len(hits) != 0 {
		t.Fatalf("mixed models: %+v %v", hits, err)
	}
	status, err = changed.Status(ctx, "a", "p", 20, 0)
	if err != nil || status[0].Status != "pending" {
		t.Fatalf("model status: %+v %v", status, err)
	}
	if _, err = db.Exec(`UPDATE requirements SET text='Новая версия',version=2 WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM requirement_chunks WHERE requirement_id='r'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale vectors: %d %v", count, err)
	}
	if _, err = s.Index(ctx, "a", "p", "r"); err != nil {
		t.Fatal(err)
	}
	hits, err = s.Search(ctx, "a", "p", "новая", 20)
	if err != nil || len(hits) != 1 || hits[0].Version != 2 || hits[0].SourceText != "Новая версия" {
		t.Fatalf("version: %+v %v", hits, err)
	}
	if _, err = db.Exec(`DELETE FROM requirements WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	hits, err = s.Search(ctx, "a", "p", "новая", 20)
	if err != nil || len(hits) != 0 {
		t.Fatalf("deleted vectors: %+v %v", hits, err)
	}
}

func TestIndexFailureRetryAndLeaseRecovery(t *testing.T) {
	ctx := context.Background()
	db := ragDB(t)
	embedder := &fakeEmbedder{profile: "model:a", fail: true}
	s := New(db, embedder)
	state, err := s.Index(ctx, "a", "p", "r")
	if err != nil || state.Status != "failed" || state.ErrorCode != "EMBEDDING_FAILED" {
		t.Fatalf("failure: %+v %v", state, err)
	}
	embedder.fail = false
	state, err = s.Index(ctx, "a", "p", "r")
	if err != nil || state.Status != "ready" || embedder.calls != 2 {
		t.Fatalf("retry: %+v calls%d %v", state, embedder.calls, err)
	}
	if _, err = db.Exec(`UPDATE requirement_index SET status='pending',attempt='expired',lease_until=clock_timestamp()-interval '1 minute' WHERE requirement_id='r'`); err != nil {
		t.Fatal(err)
	}
	state, err = s.Index(ctx, "a", "p", "r")
	if err != nil || state.Status != "ready" || embedder.calls != 3 {
		t.Fatalf("lease retry: %+v %v", state, err)
	}
}

func TestIndexConcurrentAndStaleVersionDiscard(t *testing.T) {
	ctx := context.Background()
	db := ragDB(t)
	embedder := &fakeEmbedder{profile: "model:a", started: make(chan struct{}), release: make(chan struct{})}
	s := New(db, embedder)
	type result struct {
		state State
		err   error
	}
	done := make(chan result, 1)
	go func() { state, err := s.Index(ctx, "a", "p", "r"); done <- result{state, err} }()
	<-embedder.started
	second, err := s.Index(ctx, "a", "p", "r")
	if err != nil || second.Status != "pending" || embedder.calls != 1 {
		close(embedder.release)
		t.Fatalf("duplicate job: %+v %v", second, err)
	}
	if _, err = db.Exec(`UPDATE requirements SET text='changed during embedding',version=2 WHERE id='r'`); err != nil {
		close(embedder.release)
		t.Fatal(err)
	}
	close(embedder.release)
	first := <-done
	if first.err != nil || first.state.Status != "pending" || first.state.Version != 2 {
		t.Fatalf("stale completed: %+v %v", first.state, first.err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM requirement_chunks`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale chunks: %d %v", count, err)
	}
}

func TestRagDisabledAndValidation(t *testing.T) {
	ctx := context.Background()
	db := ragDB(t)
	disabled := New(db, nil)
	status, err := disabled.Status(ctx, "a", "p", 20, 0)
	if err != nil || status[0].ErrorCode != "NOT_CONFIGURED" {
		t.Fatalf("disabled: %+v %v", status, err)
	}
	if _, err = disabled.Index(ctx, "a", "p", "r"); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if _, err = disabled.Search(ctx, "b", "p", "text", 5); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	fake := &fakeEmbedder{profile: "model:a"}
	s := New(db, fake)
	if _, err = s.Search(ctx, "a", "p", " ", 5); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err = s.Search(ctx, "a", "p", "text", 21); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if fake.calls != 0 {
		t.Fatal("invalid search called provider")
	}
}

func TestDeterministicSearchRanking(t *testing.T) {
	ctx := context.Background()
	db := ragDB(t)
	s := New(db, &fakeEmbedder{profile: "fake:ranking"})
	if _, err := db.Exec(`INSERT INTO requirements VALUES('banana','a','p','banana inventory',1,'later','later')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"r", "banana"} {
		if _, err := s.Index(ctx, "a", "p", id); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ query, id string }{{"оплата", "r"}, {"banana", "banana"}} {
		hits, err := s.Search(ctx, "a", "p", test.query, 1)
		if err != nil || len(hits) != 1 || hits[0].RequirementID != test.id || hits[0].Score < 0.99 {
			t.Fatalf("%q: %+v %v", test.query, hits, err)
		}
	}
}

func TestRealOllamaRetrievalSmoke(t *testing.T) {
	if os.Getenv("OLLAMA_SMOKE") != "1" {
		t.Skip("opt in with OLLAMA_SMOKE=1, OLLAMA_URL/MODEL/DIGEST and TEST_DATABASE_URL")
	}
	provider, err := NewOllama(os.Getenv("OLLAMA_URL"), os.Getenv("OLLAMA_MODEL"), os.Getenv("OLLAMA_DIGEST"))
	if err != nil {
		t.Fatal(err)
	}
	db := ragDB(t)
	if _, err = db.Exec(`INSERT INTO projects VALUES('smoke','a','Smoke','G','','now','now');INSERT INTO requirements VALUES('password','a','smoke','Пользователь может восстановить пароль по электронной почте.',1,'now','now'),('coffee','a','smoke','Расход кофе учитывается в граммах.',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	s := New(db, provider)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for _, id := range []string{"password", "coffee"} {
		state, err := s.Index(ctx, "a", "smoke", id)
		if err != nil || state.Status != "ready" {
			t.Fatalf("index %s: %+v %v", id, state, err)
		}
	}
	hits, err := s.Search(ctx, "a", "smoke", "Пользователь может восстановить пароль по электронной почте.", 1)
	if err != nil || len(hits) != 1 || hits[0].RequirementID != "password" {
		t.Fatalf("expected password source: %+v %v", hits, err)
	}
}
