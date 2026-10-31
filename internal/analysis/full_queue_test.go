package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func replaceQueueRequirements(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM requirements WHERE workspace='a' AND project_id='p'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO requirements SELECT 'requirement-'||lpad(n::text,4,'0'),'a','p','Requirement '||n,1,'now','now' FROM generate_series(1,$1::int) n`, count); err != nil {
		t.Fatal(err)
	}
}

func waitFullQueueSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("full queue preparation did not reach inference")
	}
}

func TestFullQueueCoverage100And200(t *testing.T) {
	for _, count := range []int{100, 200} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db, _, embed := analysisDB(t)
			replaceQueueRequirements(t, db, count)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer close(release)
			gen := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
				once.Do(func() { close(entered) })
				select {
				case <-release:
					return findingFor(pair), nil
				case <-ctx.Done():
					return Finding{}, ctx.Err()
				}
			})
			service := testService(t, db, nil, gen, "")
			run, err := service.Start(context.Background(), "a", "p")
			if err != nil {
				t.Fatal(err)
			}
			waitFullQueueSignal(t, entered)
			progress, err := service.Latest(context.Background(), "a", "p")
			expected := count * (count - 1) / 2
			if err != nil || progress.TotalPairs == nil || *progress.TotalPairs != expected {
				t.Fatalf("full coverage must include exactly %d pairs, not retrieval candidates: %+v %v", expected, progress, err)
			}
			var persisted, uniquePairs, reversed int
			err = db.QueryRow(`SELECT count(*),count(DISTINCT (left_source->>'id',right_source->>'id')),count(*) FILTER (WHERE left_source->>'id'>=right_source->>'id') FROM analysis_pairs WHERE run_id=$1`, run.ID).Scan(&persisted, &uniquePairs, &reversed)
			if err != nil || persisted != expected || uniquePairs != expected || reversed != 0 {
				t.Fatalf("normalized canonical queue: persisted=%d unique=%d reversed=%d err=%v", persisted, uniquePairs, reversed, err)
			}
			if embed.calls.Load() != 0 {
				t.Fatal("full pair enumeration depends on embeddings")
			}
			var inlinePairs int
			if err = db.QueryRow(`SELECT jsonb_array_length(pairs) FROM analysis_runs WHERE id=$1`, run.ID).Scan(&inlinePairs); err != nil || inlinePairs != 0 {
				t.Fatalf("quadratic queue retained in run JSONB: count=%d err=%v", inlinePairs, err)
			}
		})
	}
}

func TestFullQueueCrossRunReuseAndIncrementalChanges(t *testing.T) {
	db, _, embed := analysisDB(t)
	replaceQueueRequirements(t, db, 4)
	generator := &fakeGenerator{profile: "full-profile-v1"}
	service := testService(t, db, nil, generator, "")
	compare := func(wantCalls, wantPairs int) Run {
		t.Helper()
		run, err := service.startAndWait(t, context.Background(), "a", "p")
		if err != nil || run.Status != "completed" || run.TotalPairs == nil || *run.TotalPairs != wantPairs || int(generator.calls.Load()) != wantCalls {
			t.Fatalf("expected %d cumulative inference calls and %d pairs, got calls=%d run=%+v err=%v", wantCalls, wantPairs, generator.calls.Load(), run, err)
		}
		return run
	}
	original := compare(6, 6)
	repeated := compare(6, 6)
	if repeated.ID == original.ID {
		t.Fatal("explicit analysis must have independent history")
	}
	if _, err := db.Exec(`UPDATE requirements SET text='Changed one source',version=2 WHERE id='requirement-0002'`); err != nil {
		t.Fatal(err)
	}
	compare(9, 6) // Only the three pairs containing version 2 require inference.
	if _, err := db.Exec(`INSERT INTO requirements VALUES('requirement-0005','a','p','Added requirement',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	compare(13, 10) // Four new comparisons, including the changed source.
	if _, err := db.Exec(`DELETE FROM requirements WHERE id='requirement-0003'`); err != nil {
		t.Fatal(err)
	}
	compare(13, 6)
	if embed.calls.Load() != 0 {
		t.Fatal("full analysis called embedding provider")
	}
	var historical int
	if err := db.QueryRow(`SELECT count(*) FROM analysis_pairs WHERE run_id=$1 AND status='completed'`, original.ID).Scan(&historical); err != nil || historical != 6 {
		t.Fatalf("editing/deleting a source destroyed successful historical checks: count=%d err=%v", historical, err)
	}
}

func TestFullQueueReuseAllVerdictsAndProfileIsolation(t *testing.T) {
	for _, verdict := range []string{"collision", "compatible", "insufficient"} {
		t.Run(verdict, func(t *testing.T) {
			db, _, _ := analysisDB(t)
			var calls atomic.Int32
			generator := generatorFunc(func(_ context.Context, pair Pair) (Finding, error) {
				calls.Add(1)
				finding := findingFor(pair)
				finding.Verdict = verdict
				return finding, nil
			})
			service := testService(t, db, nil, generator, "embedding-old")
			original, err := service.startAndWait(t, context.Background(), "a", "p")
			if err != nil || original.Status != "completed" || calls.Load() != 1 {
				t.Fatalf("first comparison: %+v %v calls=%d", original, err, calls.Load())
			}
			// Embeddings are provenance of preparation, not identity of comparison.
			changedEmbedding := testService(t, db, nil, generator, "embedding-new")
			reused, err := changedEmbedding.startAndWait(t, context.Background(), "a", "p")
			if err != nil || reused.Status != "completed" || calls.Load() != 1 {
				t.Fatalf("embedding drift invalidated exact comparison: %+v %v calls=%d", reused, err, calls.Load())
			}
			profile := &fakeGenerator{profile: "different-immutable-profile"}
			changedModel := testService(t, db, nil, profile, "embedding-new")
			fresh, err := changedModel.startAndWait(t, context.Background(), "a", "p")
			if err != nil || fresh.Status != "completed" || profile.calls.Load() != 1 {
				t.Fatalf("model profile reused incompatible result: %+v %v calls=%d", fresh, err, profile.calls.Load())
			}
			if _, err = changedModel.Latest(context.Background(), "b", "p"); err != ErrNotFound {
				t.Fatalf("cross-workspace history leaked: %v", err)
			}
			var oldCount int
			if err = db.QueryRow(`SELECT count(*) FROM analysis_pairs WHERE run_id=$1 AND verdict=$2 AND status='completed'`, original.ID, verdict).Scan(&oldCount); err != nil || oldCount != 1 {
				t.Fatalf("old model history destroyed: %d %v", oldCount, err)
			}
		})
	}
}

func TestFullQueueRejectsLateResultWhenSourceChanges(t *testing.T) {
	db, _, _ := analysisDB(t)
	generator := &fakeGenerator{profile: "profile", started: make(chan struct{}, 1), release: make(chan struct{})}
	service := testService(t, db, nil, generator, "")
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, generator.started)
	if _, err = db.Exec(`UPDATE requirements SET version=2,text='Changed during inference' WHERE id='right'`); err != nil {
		t.Fatal(err)
	}
	close(generator.release)
	done := waitForRun(t, service, "a", "p", run.ID)
	if done.Status != "failed" || done.ErrorCode != "SOURCE_CHANGED" {
		t.Fatalf("late response accepted: %+v", done)
	}
	var accepted int
	if err = db.QueryRow(`SELECT count(*) FROM analysis_pairs WHERE run_id=$1 AND status='completed'`, run.ID).Scan(&accepted); err != nil || accepted != 0 {
		t.Fatalf("stale response cached: count=%d err=%v", accepted, err)
	}
}

// Keep this test independent of per-page fields: aggregate coverage must remain
// exact even when the compatibility snapshot intentionally returns only a page.
func TestFullQueueRunTotalsAreNotCurrentPageSize(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 12)
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, nil, generator, "")
	deadline := time.Now().Add(10 * time.Second)
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		current, readErr := service.Latest(context.Background(), "a", "p")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if current.ID == run.ID && current.Status != "running" {
			if current.Status != "completed" || current.TotalPairs == nil || *current.TotalPairs != 66 || current.ProcessedPairs != 66 || generator.calls.Load() != 66 {
				t.Fatalf("aggregate coverage truncated to a page: %+v calls=%d", current, generator.calls.Load())
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("full queue did not finish")
}

func TestFullQueueReuseSurvivesStaleWholeRun(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 3)
	var calls atomic.Int32
	second, release := make(chan struct{}), make(chan struct{})
	generator := generatorFunc(func(ctx context.Context, pair Pair) (Finding, error) {
		if calls.Add(1) == 2 {
			close(second)
			select {
			case <-release:
			case <-ctx.Done():
				return Finding{}, ctx.Err()
			}
		}
		return findingFor(pair), nil
	})
	service := testService(t, db, nil, generator, "")
	first, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, second)
	if _, err = db.Exec(`UPDATE requirements SET version=2,text='Changed third source' WHERE id='requirement-0003'`); err != nil {
		t.Fatal(err)
	}
	close(release)
	stale := waitForRun(t, service, "a", "p", first.ID)
	if !stale.Stale || stale.Status != "failed" {
		t.Fatalf("expected historical stale run: %+v", stale)
	}
	fresh, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || fresh.Status != "completed" || calls.Load() != 4 {
		t.Fatalf("stale parent run discarded unchanged successful pair: calls=%d run=%+v err=%v", calls.Load(), fresh, err)
	}
}

func TestFullQueuePagesAndHistoryAuthorizeAndPreserveProvenance(t *testing.T) {
	db, _, _ := analysisDB(t)
	replaceQueueRequirements(t, db, 4)
	generator := &fakeGenerator{profile: "profile"}
	service := testService(t, db, nil, generator, "")
	original, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ReusedPairs != 6 || repeated.NewPairs != 0 || repeated.CompletedPairs != 6 || repeated.PendingPairs != 0 || repeated.RunningPairs != 0 || repeated.FailedPairs != 0 || repeated.CompatiblePairs != 6 {
		t.Fatalf("wrong run-wide reuse aggregates: %+v", repeated)
	}
	seen := map[string]bool{}
	originals, err := service.Pairs(context.Background(), "a", "p", original.ID, 100, 0, "", "")
	if err != nil || len(originals) != 6 {
		t.Fatalf("historical page: %+v %v", originals, err)
	}
	byID := map[string]PairResult{}
	for _, pair := range originals {
		byID[pair.ID] = pair
	}
	for offset := 0; offset < 6; offset += 2 {
		page, err := service.Pairs(context.Background(), "a", "p", repeated.ID, 2, offset, "completed", "compatible")
		if err != nil || len(page) != 2 {
			t.Fatalf("page offset %d: %+v %v", offset, page, err)
		}
		for _, pair := range page {
			if seen[pair.ID] {
				t.Fatal("duplicate across pages", pair.ID)
			}
			seen[pair.ID] = true
			source, ok := byID[pair.ReusedFrom]
			if !ok || pair.CheckedAt == "" || pair.CheckedAt != source.CheckedAt || pair.GeneratorProfile != "profile" || pair.Left != source.Left || pair.Right != source.Right || pair.Explanation != source.Explanation {
				t.Fatalf("reused result lost exact identity/provenance: pair=%+v source=%+v", pair, source)
			}
		}
	}
	for _, scope := range []struct{ workspace, project string }{{"b", "p"}, {"a", "q"}} {
		if _, err = service.Pairs(context.Background(), scope.workspace, scope.project, original.ID, 2, 0, "", ""); err != ErrNotFound {
			t.Fatalf("page leaked across scope %+v: %v", scope, err)
		}
	}
	if _, err = service.Pairs(context.Background(), "a", "p", original.ID, 101, 0, "", ""); err == nil {
		t.Fatal("unbounded pair page accepted")
	}
	if _, err = service.Pairs(context.Background(), "a", "p", original.ID, 2, -1, "", ""); err == nil {
		t.Fatal("negative page offset accepted")
	}
	history, err := service.History(context.Background(), "a", "p", 1, 0)
	if err != nil || len(history) != 1 || history[0].ID != repeated.ID {
		t.Fatalf("latest history page: %+v %v", history, err)
	}
	history, err = service.History(context.Background(), "a", "p", 1, 1)
	if err != nil || len(history) != 1 || history[0].ID != original.ID {
		t.Fatalf("old history page: %+v %v", history, err)
	}
	old, err := service.Get(context.Background(), "a", "p", original.ID)
	if err != nil || old == nil || old.ID != original.ID || old.GeneratorProfile != "profile" {
		t.Fatalf("historical run: %+v %v", old, err)
	}
	if _, err = service.History(context.Background(), "b", "p", 2, 0); err != ErrNotFound {
		t.Fatalf("history leaked across workspace: %v", err)
	}
}

type mutableProfileGenerator struct {
	profile atomic.Value
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (g *mutableProfileGenerator) Profile() string { return g.profile.Load().(string) }
func (g *mutableProfileGenerator) Compare(ctx context.Context, pair Pair) (Finding, error) {
	if g.calls.Add(1) == 1 {
		close(g.entered)
		select {
		case <-g.release:
		case <-ctx.Done():
			return Finding{}, ctx.Err()
		}
	}
	return findingFor(pair), nil
}

func TestFullQueueProfileChangeDuringInferenceRejectsLateResult(t *testing.T) {
	db, _, _ := analysisDB(t)
	generator := &mutableProfileGenerator{entered: make(chan struct{}), release: make(chan struct{})}
	generator.profile.Store("profile-before")
	service := testService(t, db, nil, generator, "")
	run, err := service.Start(context.Background(), "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, generator.entered)
	generator.profile.Store("profile-after")
	close(generator.release)
	old := waitForRun(t, service, "a", "p", run.ID)
	if old.Status != "failed" || old.CompletedPairs != 0 || old.GeneratorProfile != "profile-before" {
		t.Fatalf("profile drift accepted old inference: %+v", old)
	}
	fresh, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || fresh.Status != "completed" || fresh.GeneratorProfile != "profile-after" || fresh.ReusedPairs != 0 || generator.calls.Load() != 2 {
		t.Fatalf("new profile reused late answer: %+v calls=%d err=%v", fresh, generator.calls.Load(), err)
	}
}
