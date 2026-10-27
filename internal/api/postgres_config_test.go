package api

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectSQLiteConfiguration(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "old.db"), map[string]string{"token-a": "a"})
	if err == nil {
		s.Close()
		t.Fatal("SQLite path accepted; require explicit PostgreSQL URL")
	}
	if !strings.Contains(err.Error(), "PostgreSQL") {
		t.Fatalf("unclear configuration error: %v", err)
	}
}
