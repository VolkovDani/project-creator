package api

import (
	"github.com/VolkovDani/project-creator/internal/storage"
	"testing"
)

func TestRAGStoragePrepared(t *testing.T) {
	db, err := storage.Open(testDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM requirement_chunks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("new index contains %d chunks", n)
	}
}
