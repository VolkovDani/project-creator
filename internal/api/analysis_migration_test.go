package api

import "testing"

func TestAnalysisRunStoragePrepared(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM analysis_runs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("new analysis history contains %d runs", count)
	}
}
