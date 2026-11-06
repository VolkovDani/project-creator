package summary

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type summaryHardwareFixture struct {
	id, text string
}

func paddedSummaryHardwareSource(core string) string {
	const context = "Дополнительная справка: этот повторяющийся контекст не меняет требование и не добавляет новых условий. "
	text := core
	for len(text) <= 350 {
		text += context
	}
	return text
}

func hasAllSummaryRoots(text string, roots ...string) bool {
	text = strings.ToLower(text)
	for _, root := range roots {
		if !strings.Contains(text, strings.ToLower(root)) {
			return false
		}
	}
	return true
}

func validPaymentSummary(text, allowedRoot, deniedRoot string) bool {
	lower := strings.ToLower(text)
	if !hasAllSummaryRoots(lower, allowedRoot, deniedRoot) {
		return false
	}
	only := regexp.MustCompile(`(?:только|исключительно)[^.!?]{0,60}` + allowedRoot)
	denied := regexp.MustCompile(`(?:` + deniedRoot + `[^.!?]{0,60}(?:запрещ|недопуст|нельзя|запрет)|(?:запрещ|недопуст|нельзя|запрет)[^.!?]{0,60}` + deniedRoot + `)`)
	return only.MatchString(lower) && denied.MatchString(lower)
}

func validShipmentSummary(text string) bool {
	lower := strings.ToLower(text)
	return (strings.Contains(lower, "смс") || strings.Contains(lower, "sms") || strings.Contains(lower, "текстов")) &&
		(strings.Contains(lower, "отгруз") || strings.Contains(lower, "отправ") || strings.Contains(lower, "уведом") || strings.Contains(lower, "сообщ"))
}

type fullSourceNPUGenerator struct {
	next     analysis.Generator
	expected map[string]analysis.Source
	checked  atomic.Int32
	bad      atomic.Bool
}

func (g *fullSourceNPUGenerator) Profile() string { return g.next.Profile() }
func (g *fullSourceNPUGenerator) Provenance() string {
	if p, ok := g.next.(interface{ Provenance() string }); ok {
		return p.Provenance()
	}
	return g.next.Profile()
}
func (g *fullSourceNPUGenerator) Compare(ctx context.Context, pair analysis.Pair) (analysis.Finding, error) {
	g.checked.Add(1)
	left, leftOK := g.expected[pair.Left.ID]
	right, rightOK := g.expected[pair.Right.ID]
	if !leftOK || !rightOK || pair.Left != left || pair.Right != right || pair.Preclassification == nil {
		g.bad.Store(true)
	}
	return g.next.Compare(ctx, pair)
}

func TestRealSummaryLayaNPUFullSourcePipeline(t *testing.T) {
	required := []string{
		"SUMMARY_PIPELINE_SMOKE_URL", "SUMMARY_PIPELINE_SMOKE_MODEL", "SUMMARY_PIPELINE_SMOKE_DIGEST",
		"PIPELINE_SMOKE_URL", "PIPELINE_SMOKE_LAYA_MODEL", "PIPELINE_SMOKE_LAYA_DIGEST",
		"PROJECT_CREATOR_NPU_SOCKET", "PROJECT_CREATOR_NPU_PROFILE", "TEST_DATABASE_URL",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			if key == "SUMMARY_PIPELINE_SMOKE_URL" {
				t.Skip("set SUMMARY_PIPELINE_SMOKE_URL plus local summary/Laya/NPU pins and TEST_DATABASE_URL to opt in")
			}
			t.Fatalf("SUMMARY_PIPELINE_SMOKE_URL is set; %s must also be explicitly configured", key)
		}
	}
	if os.Getenv("PIPELINE_SMOKE_LAYA_MODEL") != "laya:322m-multilingual-bf16" {
		t.Fatal("PIPELINE_SMOKE_LAYA_MODEL must name the pinned multilingual Laya adapter model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	db := testDB(t)
	if _, err := db.ExecContext(ctx, `DELETE FROM requirements WHERE workspace='a' AND project_id='p'`); err != nil {
		t.Fatal(err)
	}
	const filler = "Дополнительная справка: этот повторяющийся контекст не меняет требование и не добавляет новых условий. "
	fixtures := []summaryHardwareFixture{
		{"summary-smoke-card", paddedSummaryHardwareSource("Для каждого заказа разрешена оплата только банковской картой. Оплата наличными запрещена. " + filler)},
		{"summary-smoke-cash", paddedSummaryHardwareSource("Для каждого заказа разрешена оплата только наличными. Оплата банковской картой запрещена. " + filler)},
		{"summary-smoke-shipment", paddedSummaryHardwareSource("При отгрузке заказа со склада система отправляет клиенту SMS-уведомление. " + filler)},
	}
	expected := make(map[string]analysis.Source, len(fixtures))
	for _, fixture := range fixtures {
		if len(fixture.text) <= 350 || len(fixture.text) > 800 || !utf8.ValidString(fixture.text) {
			t.Fatalf("fixture %s has invalid byte length %d", fixture.id, len(fixture.text))
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO requirements(workspace,project_id,id,text,version,created_at,updated_at) VALUES('a','p',$1,$2,1,clock_timestamp(),clock_timestamp())`, fixture.id, fixture.text); err != nil {
			t.Fatal(err)
		}
		expected[fixture.id] = analysis.Source{ID: fixture.id, Version: 1, Text: fixture.text}
	}

	summarizer, err := analysis.NewSummaryOllama(os.Getenv("SUMMARY_PIPELINE_SMOKE_URL"), os.Getenv("SUMMARY_PIPELINE_SMOKE_MODEL"), os.Getenv("SUMMARY_PIPELINE_SMOKE_DIGEST"))
	if err != nil {
		t.Fatal(err)
	}
	laya, err := analysis.NewLaya(os.Getenv("PIPELINE_SMOKE_URL"), os.Getenv("PIPELINE_SMOKE_LAYA_MODEL"), os.Getenv("PIPELINE_SMOKE_LAYA_DIGEST"))
	if err != nil {
		t.Fatal(err)
	}
	npu, err := analysis.NewOpenVINO(os.Getenv("PROJECT_CREATOR_NPU_SOCKET"), os.Getenv("PROJECT_CREATOR_NPU_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}

	summaries := New(db, summarizer)
	if err = summaries.Start([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	defer summaries.Close()
	deadline := time.Now().Add(5 * time.Minute)
	states := make(map[string]State, len(fixtures))
	for _, fixture := range fixtures {
		for {
			rows, readErr := summaries.ByIDs(ctx, "a", "p", []string{fixture.id})
			if readErr != nil {
				t.Fatal(readErr)
			}
			state := rows[0]
			if state.Status == "ready" {
				if len(state.Text) == 0 || len(state.Text) > 350 || !utf8.ValidString(state.Text) {
					t.Fatalf("invalid generated summary %s: %d UTF-8 bytes", fixture.id, len(state.Text))
				}
				states[fixture.id] = state
				break
			}
			if state.Status == "failed" || state.Status == "cancelled" || state.Status == "stale" {
				t.Fatalf("summary generation failed for %s: status=%s error=%s", fixture.id, state.Status, state.ErrorCode)
			}
			if time.Now().After(deadline) {
				t.Fatalf("summary generation exceeded five minutes for %s; last state=%s error=%s", fixture.id, state.Status, state.ErrorCode)
			}
			select {
			case <-ctx.Done():
				t.Fatalf("summary generation context ended for %s: %v", fixture.id, ctx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	for _, fixture := range fixtures {
		state := states[fixture.id]
		encoded, marshalErr := json.Marshal(map[string]any{
			"id": fixture.id, "original": fixture.text, "original_utf8_bytes": len(fixture.text),
			"summary": state.Text, "summary_utf8_bytes": len(state.Text), "profile": state.Profile,
			"provenance": state.Provenance, "status": state.Status,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		t.Log(string(encoded))
	}
	if !validPaymentSummary(states[fixtures[0].id].Text, "карт", "налич") || !validPaymentSummary(states[fixtures[1].id].Text, "налич", "карт") {
		t.Fatalf("summaries did not preserve payment exclusivity/negation: card=%q cash=%q", states[fixtures[0].id].Text, states[fixtures[1].id].Text)
	}
	if !validShipmentSummary(states[fixtures[2].id].Text) {
		t.Fatalf("shipment summary did not preserve event and SMS channel: %q", states[fixtures[2].id].Text)
	}

	trace := &smokeTrace{firstNPU: make(chan struct{}, 1)}
	checkedNPU := &fullSourceNPUGenerator{next: npu, expected: expected}
	generator := &smokeGenerator{next: checkedNPU, trace: trace}
	classifier := &smokeClassifier{next: summaries.Classifier(laya), trace: trace}
	analyzer := analysis.New(db, nil, generator, "", classifier)
	defer analyzer.Close()
	run, err := analyzer.StartWithOptions(ctx, "a", "p", analysis.Options{Mode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	for {
		current, getErr := analyzer.Get(ctx, "a", "p", run.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current.Status != "running" {
			run = *current
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("summary-to-NPU pipeline exceeded eight minutes: %v", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	if run.Status != "completed" || run.CompletedPairs != 3 || run.FailedPairs != 0 {
		t.Fatalf("pipeline did not complete all pairs: status=%s complete=%d failed=%d error=%s", run.Status, run.CompletedPairs, run.FailedPairs, run.ErrorCode)
	}
	if checkedNPU.bad.Load() || checkedNPU.checked.Load() != 3 {
		t.Fatalf("final generator did not receive the exact full source snapshot and Laya result for all pairs: checked=%d bad=%v", checkedNPU.checked.Load(), checkedNPU.bad.Load())
	}
	var verdict string
	var leftVersion, rightVersion int
	if err = db.QueryRowContext(ctx, `SELECT verdict,(left_source->>'version')::int,(right_source->>'version')::int FROM analysis_pairs WHERE run_id=$1 AND ((left_source->>'id'=$2 AND right_source->>'id'=$3) OR (left_source->>'id'=$3 AND right_source->>'id'=$2))`, run.ID, fixtures[0].id, fixtures[1].id).Scan(&verdict, &leftVersion, &rightVersion); err != nil {
		t.Fatal(err)
	}
	if verdict != "collision" || leftVersion != 1 || rightVersion != 1 {
		t.Fatalf("card/cash finding did not preserve expected semantic result and source versions: verdict=%s versions=%d,%d", verdict, leftVersion, rightVersion)
	}
	var layaCompleted, validFindings int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE laya_status='completed'),count(*) FILTER(WHERE status='completed' AND verdict IN ('collision','compatible','insufficient')) FROM analysis_pairs WHERE run_id=$1`, run.ID).Scan(&layaCompleted, &validFindings); err != nil {
		t.Fatal(err)
	}
	if layaCompleted != 3 || validFindings != 3 {
		t.Fatalf("pipeline result rows incomplete or invalid: laya=%d findings=%d", layaCompleted, validFindings)
	}
	t.Logf("real summary+Laya+NPU pipeline complete: pairs=%d summary_profile=%s laya_profile=%s npu_profile=%s", validFindings, summarizer.Profile(), laya.Profile(), npu.Profile())
}
