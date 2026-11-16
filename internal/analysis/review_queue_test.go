package analysis

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func reviewLongSource(seed string) string {
	var out strings.Builder
	for i := 0; i < 2000; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, i)))
		out.WriteRune(0x4e00 + rune(int(sum[0])*256+int(sum[1]))%20000)
	}
	return out.String()
}

func TestQueueAcceptsLongPoorlyCompressibleSources(t *testing.T) {
	db, _, _ := analysisDB(t)
	left, right := reviewLongSource("left"), reviewLongSource("right")
	if _, err := db.Exec(`UPDATE requirements SET text=CASE id WHEN 'left' THEN $1 ELSE $2 END WHERE workspace='a' AND project_id='p'`, left, right); err != nil {
		t.Fatal(err)
	}
	generator := &fakeGenerator{profile: "long-snapshot-profile"}
	service := testService(t, db, nil, generator, "")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.Status != "completed" || run.TotalPairs == nil || *run.TotalPairs != 1 || generator.calls.Load() != 1 {
		t.Fatalf("valid long sources must remain comparable: status=%s error=%s pairs=%v calls=%d err=%v", run.Status, run.ErrorCode, run.TotalPairs, generator.calls.Load(), err)
	}
	pairs, err := service.Pairs(context.Background(), "a", "p", run.ID, 100, 0, "", "")
	if err != nil || len(pairs) != 1 || pairs[0].Left.Text != left || pairs[0].Right.Text != right {
		t.Fatalf("long source snapshots were truncated or lost: count=%d err=%v", len(pairs), err)
	}
}

func TestPreparationRollbackDoesNotAdvanceDurableCursor(t *testing.T) {
	db, _, _ := analysisDB(t)
	if _, err := db.Exec(`CREATE FUNCTION reject_review_pair() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'review transient preparation failure'; END $$; CREATE TRIGGER reject_review_pair BEFORE INSERT ON analysis_pairs FOR EACH ROW EXECUTE FUNCTION reject_review_pair()`); err != nil {
		t.Fatal(err)
	}
	generator := &fakeGenerator{profile: "rollback-profile"}
	service := testService(t, db, nil, generator, "")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.Status != "failed" || generator.calls.Load() != 0 {
		t.Fatalf("preparation failure must stop before inference: status=%s calls=%d err=%v", run.Status, generator.calls.Load(), err)
	}
	var left, right string
	if err = db.QueryRow(`SELECT pair_cursor_left,pair_cursor_right FROM analysis_runs WHERE id=$1`, run.ID).Scan(&left, &right); err != nil {
		t.Fatal(err)
	}
	if left != "" || right != "" || run.TotalPairs != nil {
		t.Errorf("rolled-back preparation committed a cursor or total: left=%q right=%q total=%v", left, right, run.TotalPairs)
	}
	if _, err = db.Exec(`DROP TRIGGER reject_review_pair ON analysis_pairs`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Retry(context.Background(), "a", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	resumed := waitForRun(t, service, "a", "p", run.ID)
	if resumed.Status != "completed" || resumed.TotalPairs == nil || *resumed.TotalPairs != 1 || resumed.CompletedPairs != 1 || generator.calls.Load() != 1 {
		t.Fatalf("retry skipped uncommitted pairs: status=%s pairs=%v completed=%d calls=%d", resumed.Status, resumed.TotalPairs, resumed.CompletedPairs, generator.calls.Load())
	}
}

func TestStartReturnsPersistedSourcePageBeforeWorkerClaims(t *testing.T) {
	db, _, _ := analysisDB(t)
	s := &Service{db: db, generator: &fakeGenerator{profile: "source-page"}}
	r, err := s.Start(context.Background(), "a", "p")
	if err != nil || len(r.Sources) != 2 || r.Sources[0].ID != "left" || r.Sources[1].ID != "right" || r.Sources[0].Version != 1 {
		t.Fatalf("start response lost durable source page: %+v %v", r.Sources, err)
	}
}
