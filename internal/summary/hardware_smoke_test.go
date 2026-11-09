package summary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/rag"
)

// This test is deliberately opt-in. It uses only explicitly configured local
// Ollama and NPU runtimes and writes fixtures into testDB's temporary schema.
// It measures overlap and elapsed time; it does not claim an SLA or model quality.
type smokeInterval struct {
	Stage string    `json:"stage"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Error string    `json:"error,omitempty"`
}

type smokeSearchMetric struct {
	Number                int   `json:"number"`
	Warm                  bool  `json:"warm"`
	WaitBeforeEmbedMS     int64 `json:"wait_before_embed_ms"`
	EmbeddingDurationMS   int64 `json:"embedding_duration_ms"`
	RetrievalAfterEmbedMS int64 `json:"retrieval_after_embed_ms"`
	TotalSearchDurationMS int64 `json:"total_search_duration_ms"`
	HitCount              int   `json:"hit_count"`
}

type smokeTrace struct {
	mu        sync.Mutex
	intervals []smokeInterval
	firstNPU  chan struct{}
}

func (t *smokeTrace) record(stage string, started time.Time, err error) {
	t.mu.Lock()
	t.intervals = append(t.intervals, smokeInterval{Stage: stage, Start: started, End: time.Now(), Error: safeSmokeError(stage, err)})
	t.mu.Unlock()
}

// Provider errors are reduced to fixed adapter diagnostics; request/model text
// and raw response bodies are never included in the test log.
func safeSmokeError(stage string, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	message := err.Error()
	if stage == "embedding" {
		switch message {
		case "local embedding runtime unavailable or request cancelled", "local embedding runtime rejected request", "invalid local model response size", "invalid local model response", "embedding model digest changed; configure a new profile and reindex", "pinned embedding model is not installed locally":
			return message
		default:
			return "embedding_call_failed"
		}
	}
	if stage == "npu_generator" {
		switch message {
		case "NPU_UNAVAILABLE", "NPU_RECOVERY_FAILED", "NPU_WORKER_EXIT", "TIMEOUT", "INVALID_OUTPUT", "PROFILE_CHANGED":
			return message
		case "invalid NPU request", "invalid NPU response size", "invalid NPU response", "NPU response contains trailing data", "local NPU runtime rejected request; inspect runtime diagnostics", "incomplete or unsupported NPU response", "NPU response profile or device does not match request", "invalid NPU comparison source mapping":
			return message
		default:
			return "npu_call_failed"
		}
	}
	switch message {
	case "local analysis runtime unavailable or request cancelled", "local analysis runtime rejected request", "invalid local analysis response size", "invalid local analysis response", "analysis model digest changed; configure a new profile", "pinned analysis model is not installed locally", "Laya response model or answer set does not match request", "Laya response is missing decision answer", "Laya response does not match decision schema", "Laya scores do not match decision schema", "Laya scores must be numeric":
		return message
	default:
		return "laya_call_failed"
	}
}

type smokeEmbedder struct {
	next  rag.Embedder
	trace *smokeTrace
}

func (e *smokeEmbedder) Profile() string { return e.next.Profile() }
func (e *smokeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	started := time.Now()
	vectors, err := e.next.Embed(ctx, texts)
	e.trace.record("embedding", started, err)
	return vectors, err
}

func percentileMS(metrics []smokeSearchMetric, value func(smokeSearchMetric) int64, percentile int) int64 {
	if len(metrics) == 0 {
		return 0
	}
	values := make([]int64, len(metrics))
	for i, metric := range metrics {
		values[i] = value(metric)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := (percentile*len(values)+99)/100 - 1 // nearest-rank percentile
	return values[index]
}

func searchPercentiles(metrics []smokeSearchMetric) map[string]any {
	return map[string]any{
		"sample_count":                 len(metrics),
		"p50_wait_before_embed_ms":     percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.WaitBeforeEmbedMS }, 50),
		"p95_wait_before_embed_ms":     percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.WaitBeforeEmbedMS }, 95),
		"p50_embedding_duration_ms":    percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.EmbeddingDurationMS }, 50),
		"p95_embedding_duration_ms":    percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.EmbeddingDurationMS }, 95),
		"p50_retrieval_after_embed_ms": percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.RetrievalAfterEmbedMS }, 50),
		"p95_retrieval_after_embed_ms": percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.RetrievalAfterEmbedMS }, 95),
		"p50_total_search_duration_ms": percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.TotalSearchDurationMS }, 50),
		"p95_total_search_duration_ms": percentileMS(metrics, func(m smokeSearchMetric) int64 { return m.TotalSearchDurationMS }, 95),
	}
}

func (t *smokeTrace) snapshot() []smokeInterval {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]smokeInterval(nil), t.intervals...)
}

type smokeGenerator struct {
	next  analysis.Generator
	trace *smokeTrace
}

func (g *smokeGenerator) Profile() string { return g.next.Profile() }
func (g *smokeGenerator) Provenance() string {
	if p, ok := g.next.(interface{ Provenance() string }); ok {
		return p.Provenance()
	}
	return g.next.Profile()
}
func (g *smokeGenerator) Compare(ctx context.Context, pair analysis.Pair) (analysis.Finding, error) {
	started := time.Now()
	select {
	case g.trace.firstNPU <- struct{}{}:
	default:
	}
	finding, err := g.next.Compare(ctx, pair)
	g.trace.record("npu_generator", started, err)
	return finding, err
}

type smokeClassifier struct {
	next  analysis.Classifier
	trace *smokeTrace
}

func (c *smokeClassifier) Profile() string { return c.next.Profile() }
func (c *smokeClassifier) Provenance() string {
	if p, ok := c.next.(interface{ Provenance() string }); ok {
		return p.Provenance()
	}
	return c.next.Profile()
}
func (c *smokeClassifier) Classify(ctx context.Context, pair analysis.Pair) (analysis.Preclassification, error) {
	started := time.Now()
	result, err := c.next.Classify(ctx, pair)
	c.trace.record("laya_classifier", started, err)
	return result, err
}
func (c *smokeClassifier) ClassifyScoped(ctx context.Context, workspace, project string, pair analysis.Pair) (analysis.Preclassification, error) {
	started := time.Now()
	scoped, ok := c.next.(interface {
		ClassifyScoped(context.Context, string, string, analysis.Pair) (analysis.Preclassification, error)
	})
	if !ok {
		err := fmt.Errorf("scoped classifier is unavailable")
		c.trace.record("laya_classifier", started, err)
		return analysis.Preclassification{}, err
	}
	result, err := scoped.ClassifyScoped(ctx, workspace, project, pair)
	c.trace.record("laya_classifier", started, err)
	return result, err
}

type smokeSummary struct{}

func (smokeSummary) Profile() string { return "pipeline-hardware-smoke-verbatim-v1" }
func (smokeSummary) Provenance() string {
	return `{"adapter":"synthetic-verbatim-smoke","version":"1"}`
}
func (smokeSummary) Summarize(_ context.Context, source analysis.Source) (string, error) {
	return source.Text, nil
}

func TestRealNPUAndLayaPipelineWithConcurrentSearch(t *testing.T) {
	url := os.Getenv("PIPELINE_SMOKE_URL")
	if url == "" {
		t.Skip("set PIPELINE_SMOKE_URL and the required local model/NPU pins to opt in")
	}
	required := []string{
		"PIPELINE_SMOKE_LAYA_MODEL", "PIPELINE_SMOKE_LAYA_DIGEST",
		"PIPELINE_SMOKE_EMBEDDING_MODEL", "PIPELINE_SMOKE_EMBEDDING_DIGEST",
		"PROJECT_CREATOR_NPU_SOCKET", "PROJECT_CREATOR_NPU_PROFILE",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Fatalf("PIPELINE_SMOKE_URL is set; %s must also be explicitly configured", key)
		}
	}
	if os.Getenv("PIPELINE_SMOKE_LAYA_MODEL") != "laya:322m-multilingual-bf16" {
		t.Fatalf("PIPELINE_SMOKE_LAYA_MODEL must name the pinned multilingual Laya adapter model")
	}
	requirementCount := 3
	if raw := os.Getenv("PIPELINE_SMOKE_REQUIREMENTS"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 3 || count > 7 {
			t.Fatalf("PIPELINE_SMOKE_REQUIREMENTS must be between 3 and 7")
		}
		requirementCount = count
	}
	searchCount := 1
	if raw := os.Getenv("PIPELINE_SMOKE_SEARCHES"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 1 || count > 20 {
			t.Fatalf("PIPELINE_SMOKE_SEARCHES must be between 1 and 20")
		}
		searchCount = count
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	db := testDB(t)
	if _, err := db.ExecContext(ctx, `DELETE FROM requirements WHERE workspace='a' AND project_id='p'`); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct{ id, text string }{
		{"smoke-card", "For every order, payment is accepted only by credit card; cash payments are prohibited."},
		{"smoke-cash", "For every order, payment is accepted only in cash; card payments are prohibited."},
		{"smoke-shipment", "The system sends a text message when an order shipment is dispatched."},
		{"smoke-refund", "A refund is issued within five business days after a returned item is received."},
		{"smoke-address", "Customers can save multiple delivery addresses in their account."},
		{"smoke-inventory", "The inventory count decreases when an order is confirmed."},
		{"smoke-language", "The account page lets a customer choose the interface language."},
	}
	for _, fixture := range fixtures[:requirementCount] {
		if len(fixture.text) > 350 {
			t.Fatalf("smoke fixture %s exceeds the deterministic summary bound", fixture.id)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO requirements(workspace,project_id,id,text,version,created_at,updated_at) VALUES('a','p',$1,$2,1,clock_timestamp(),clock_timestamp())`, fixture.id, fixture.text); err != nil {
			t.Fatal(err)
		}
	}

	ollamaURL := strings.TrimRight(url, "/")
	laya, err := analysis.NewLaya(ollamaURL, os.Getenv("PIPELINE_SMOKE_LAYA_MODEL"), os.Getenv("PIPELINE_SMOKE_LAYA_DIGEST"))
	if err != nil {
		t.Fatal(err)
	}
	embedder, err := rag.NewOllama(ollamaURL, os.Getenv("PIPELINE_SMOKE_EMBEDDING_MODEL"), os.Getenv("PIPELINE_SMOKE_EMBEDDING_DIGEST"))
	if err != nil {
		t.Fatal(err)
	}
	npu, err := analysis.NewOpenVINO(os.Getenv("PROJECT_CREATOR_NPU_SOCKET"), os.Getenv("PROJECT_CREATOR_NPU_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}

	summaries := New(db, smokeSummary{})
	if err = summaries.Start([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	defer summaries.Close()
	for _, fixture := range fixtures[:requirementCount] {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			rows, readErr := summaries.ByIDs(ctx, "a", "p", []string{fixture.id})
			if readErr != nil {
				t.Fatal(readErr)
			}
			if rows[0].Status == "ready" {
				if rows[0].Text != fixture.text {
					t.Fatalf("synthetic summary did not preserve source %s", fixture.id)
				}
				break
			}
			if rows[0].Status == "failed" {
				t.Fatalf("synthetic summary failed for %s: %s", fixture.id, rows[0].ErrorCode)
			}
			time.Sleep(10 * time.Millisecond)
		}
		rows, readErr := summaries.ByIDs(ctx, "a", "p", []string{fixture.id})
		if readErr != nil || rows[0].Status != "ready" {
			t.Fatalf("summary for %s did not become ready: rows=%+v err=%v", fixture.id, rows, readErr)
		}
	}

	trace := &smokeTrace{firstNPU: make(chan struct{}, 1)}
	tracedEmbedder := &smokeEmbedder{next: embedder, trace: trace}
	retriever := rag.New(db, tracedEmbedder)
	indexStart := time.Now()
	for _, fixture := range fixtures[:requirementCount] {
		state, indexErr := retriever.Index(ctx, "a", "p", fixture.id)
		if indexErr != nil || state.Status != "ready" {
			t.Fatalf("index %s: state=%+v err=%v", fixture.id, state, indexErr)
		}
	}
	indexElapsed := time.Since(indexStart)

	classifier := &smokeClassifier{next: summaries.Classifier(laya), trace: trace}
	generator := &smokeGenerator{next: npu, trace: trace}
	analyzer := analysis.New(db, retriever, generator, embedder.Profile(), classifier)
	start := time.Now()
	var runID string
	searchMetrics := make([]smokeSearchMetric, 0, searchCount)
	defer func() {
		analyzer.Close()
		value := map[string]any{
			"test": "real-npu-laya-embedding-pipeline-smoke", "run_id": runID,
			"laya_model": os.Getenv("PIPELINE_SMOKE_LAYA_MODEL"), "laya_digest": os.Getenv("PIPELINE_SMOKE_LAYA_DIGEST"),
			"embedding_model": os.Getenv("PIPELINE_SMOKE_EMBEDDING_MODEL"), "embedding_digest": os.Getenv("PIPELINE_SMOKE_EMBEDDING_DIGEST"),
			"npu_profile_pin": os.Getenv("PROJECT_CREATOR_NPU_PROFILE"), "laya_profile": laya.Profile(),
			"npu_profile": npu.Profile(), "embedding_profile": embedder.Profile(),
			"index_elapsed_ms": indexElapsed.Milliseconds(), "search_count_requested": searchCount,
			"search_metrics": searchMetrics, "search_percentiles": searchPercentiles(searchMetrics),
			"pipeline_elapsed_ms": time.Since(start).Milliseconds(), "calls": trace.snapshot(),
		}
		if runID != "" {
			var runCode string
			if e := db.QueryRow(`SELECT error_code FROM analysis_runs WHERE id=$1`, runID).Scan(&runCode); e == nil {
				value["run_error_code"] = runCode
			}
			rows, e := db.Query(`SELECT error_code,count(*) FROM analysis_pairs WHERE run_id=$1 GROUP BY error_code ORDER BY error_code`, runID)
			if e == nil {
				counts := make(map[string]int)
				for rows.Next() {
					var code string
					var count int
					if rows.Scan(&code, &count) == nil {
						counts[code] = count
					}
				}
				rows.Close()
				value["pair_error_code_counts"] = counts
			} else {
				value["pair_error_counts_query"] = "unavailable"
			}
		}
		if encoded, e := json.Marshal(value); e == nil {
			t.Log(string(encoded))
		}
	}()
	run, err := analyzer.StartWithOptions(ctx, "a", "p", analysis.Options{Mode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	runID = run.ID
	select {
	case <-trace.firstNPU:
	case <-ctx.Done():
		t.Fatalf("NPU generator did not start: %v", ctx.Err())
	}
	for i := 0; i < searchCount; i++ {
		active, activeErr := analyzer.Get(ctx, "a", "p", run.ID)
		if activeErr != nil || active.Status != "running" {
			t.Fatalf("analysis is not running before concurrent search %d: status=%v err=%v", i+1, active, activeErr)
		}
		searchStart := time.Now()
		traceBefore := trace.snapshot()
		searchCtx, searchCancel := context.WithTimeout(ctx, 120*time.Second)
		hits, searchErr := retriever.Search(searchCtx, "a", "p", "credit card payment order", 5)
		searchCancel()
		searchEnd := time.Now()
		if searchErr != nil || len(hits) == 0 {
			t.Fatalf("concurrent local embedding search %d returned %d hits after %s: %v", i+1, len(hits), searchEnd.Sub(searchStart), searchErr)
		}
		if searchEnd.Sub(searchStart) > 120*time.Second {
			t.Fatalf("embedding search exceeded generous 120s guard: %s", searchEnd.Sub(searchStart))
		}
		traceAfter := trace.snapshot()
		var embedStart, embedEnd time.Time
		for _, call := range traceAfter[len(traceBefore):] {
			if call.Stage == "embedding" && !call.Start.Before(searchStart) && !call.End.After(searchEnd) {
				embedStart, embedEnd = call.Start, call.End
				if call.Error != "" {
					t.Fatalf("embedding provider failed during search %d: %s", i+1, call.Error)
				}
				break
			}
		}
		if embedStart.IsZero() || embedEnd.Before(embedStart) {
			t.Fatalf("search %d has no matching traced embedder interval", i+1)
		}
		searchMetrics = append(searchMetrics, smokeSearchMetric{
			Number: i + 1, Warm: i > 0,
			WaitBeforeEmbedMS:     embedStart.Sub(searchStart).Milliseconds(),
			EmbeddingDurationMS:   embedEnd.Sub(embedStart).Milliseconds(),
			RetrievalAfterEmbedMS: searchEnd.Sub(embedEnd).Milliseconds(),
			TotalSearchDurationMS: searchEnd.Sub(searchStart).Milliseconds(),
			HitCount:              len(hits),
		})
	}

	var final *analysis.Run
	for {
		final, err = analyzer.Get(ctx, "a", "p", run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status != "running" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("pipeline did not finish within 8 minutes: %v", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	if final.Status != "completed" || final.CompletedPairs != requirementCount*(requirementCount-1)/2 || final.FailedPairs != 0 {
		t.Fatalf("actual model pipeline did not complete all pairs: status=%s complete=%d failed=%d total=%d error=%s", final.Status, final.CompletedPairs, final.FailedPairs, requirementCount*(requirementCount-1)/2, final.ErrorCode)
	}
	pairCount, validFindings, classified := 0, 0, 0
	if err = db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE status='completed' AND verdict IN ('collision','compatible','insufficient')),count(*) FILTER(WHERE laya_status='completed') FROM analysis_pairs WHERE run_id=$1`, run.ID).Scan(&pairCount, &validFindings, &classified); err != nil {
		t.Fatal(err)
	}
	if pairCount != requirementCount*(requirementCount-1)/2 || validFindings != pairCount || classified != pairCount {
		t.Fatalf("invalid or incomplete model pair rows: total=%d valid=%d classified=%d", pairCount, validFindings, classified)
	}
	intervals := trace.snapshot()
	var overlap bool
	for _, a := range intervals {
		for _, b := range intervals {
			if a.Stage != b.Stage && a.Start.Before(b.End) && b.Start.Before(a.End) {
				if (a.Stage == "laya_classifier" && b.Stage == "npu_generator") || (a.Stage == "npu_generator" && b.Stage == "laya_classifier") {
					overlap = true
				}
			}
		}
	}
	if !overlap {
		t.Fatalf("no observed Laya/NPU call overlap across %d instrumented calls", len(intervals))
	}
	if final.GeneratorProfile != analyzer.Profile() || final.ClassifierProfile != classifier.Profile() {
		t.Fatalf("wrapped model provenance/profile was not preserved: generator=%s classifier=%s", final.GeneratorProfile, final.ClassifierProfile)
	}
	t.Logf("search_percentiles=%v observed_overlap=%v completed_pairs=%d", searchPercentiles(searchMetrics), overlap, final.CompletedPairs)
}
