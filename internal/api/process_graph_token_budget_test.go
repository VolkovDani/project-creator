package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

func TestModelManagerProcessGraphPreflight(t *testing.T) {
	binary := os.Getenv("MODELS_CONFIGURATOR_TEST_BIN")
	if binary == "" {
		t.Skip("set MODELS_CONFIGURATOR_TEST_BIN to run the cross-repository test")
	}
	narrow := exerciseProcessGraphPreflight(t, binary, 1200, false)
	wide := exerciseProcessGraphPreflight(t, binary, 4096, false)
	if wide < 1 || narrow <= wide {
		t.Fatalf("increasing the input window did not reduce the number of planned batches: %d → %d", narrow, wide)
	}
	t.Run("response token limit", func(t *testing.T) {
		exerciseProcessGraphPreflight(t, binary, 4096, true)
	})
}

func exerciseProcessGraphPreflight(t *testing.T, binary string, window int, exhaustFirst bool) int {
	t.Helper()
	digest := strings.Repeat("b", 64)
	socket := filepath.Join(t.TempDir(), "npu.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var counted, generated atomic.Int32
	runtime := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		binding := map[string]any{"profile": digest, "device": "NPU"}
		if r.Method == "GET" && r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(binding)
			return
		}
		var req struct {
			Profile             string              `json:"profile"`
			Messages            []map[string]string `json:"messages"`
			Schema              json.RawMessage     `json:"schema"`
			MaxCompletionTokens int                 `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Profile != digest || len(req.Messages) != 2 ||
			!strings.Contains(req.Messages[1]["content"], "Dispatcher") || len(req.Schema) == 0 || req.MaxCompletionTokens != 1024 {
			t.Errorf("unexpected private %s request: %v", r.URL.Path, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		tokens := (len(req.Messages[0]["content"]) + len(req.Messages[1]["content"]) + len(req.Schema)) / 4
		switch r.URL.Path {
		case "/v1/count-tokens":
			if generated.Load() != 0 {
				t.Error("input was still being planned after generation started")
			}
			counted.Add(1)
			binding["input_tokens"] = tokens
			binding["max_input_tokens"] = window
			binding["max_completion_tokens"] = 512
		case "/v1/generate":
			if counted.Load() == 0 || tokens > window {
				t.Errorf("generation was not planned against the active model budget: %d > %d", tokens, window)
			}
			if generated.Add(1) == 1 {
				entered <- struct{}{}
				select {
				case <-r.Context().Done():
					return
				case <-release:
				}
				if exhaustFirst {
					w.WriteHeader(http.StatusBadGateway)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "NPU generation failed", "code": "OUTPUT_TOKEN_LIMIT"})
					return
				}
			}
			binding["content"] = `{"r":[]}`
		default:
			t.Errorf("unexpected runtime route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(binding)
	})}
	go runtime.Serve(listener)
	t.Cleanup(func() { runtime.Close() })
	base, models := startTestManager(t, binary, []map[string]string{
		{"name": "graph", "provider": "openvino-npu", "base_url": "unix://" + socket, "role": "chat", "model": "stub", "digest": digest},
	}, nil)
	if len(models) != 1 {
		t.Fatalf("unexpected NPU inventory: %+v", models)
	}
	g, err := analysis.NewSummaryOpenAICompatible(base, models[0].ID, "", "en")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, g)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Keep the synthetic inputs out of the unrelated summary queue.
	if _, err := s.db.Exec(`
		DELETE FROM summary_profiles WHERE workspace='a';
		INSERT INTO projects VALUES ('p','a','P','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		SELECT 'r'||lpad(i::text,2,'0'),'a','p','Dispatcher approves shipment '||i||'. ' ||
		  repeat('The dispatcher records the validation result. ', 8),1,'now','now' FROM generate_series(1,16) i;
	`); err != nil {
		t.Fatal(err)
	}
	call := func(token, query, field string) map[string]any {
		t.Helper()
		out := summaryRequest(t, s, token, query)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		result, _ := out["data"].(map[string]any)[field].(map[string]any)
		return result
	}
	const fields = "status run { id topic total completed failed }"
	const start = `mutation {startBackgroundProcessGraph(input:{projectId:"p",keyword:"Dispatcher"}){` + fields + `}}`
	const latest = `{latestProcessGraphRun(projectId:"p"){` + fields + `}}`
	if other := call("token-b", start, "startBackgroundProcessGraph"); other["status"] != "not_found" || counted.Load() != 0 {
		t.Fatalf("foreign workspace reached the preflight: %v", other)
	}
	job := call("token-a", start, "startBackgroundProcessGraph")
	if run, ok := job["run"].(map[string]any); !ok || job["status"] != "planning" || run["total"] != float64(0) {
		t.Fatalf("start did not expose planning: %v", job)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatalf("preflight never proceeded to inference: %v", call("token-a", latest, "latestProcessGraphRun"))
	}
	active := call("token-a", latest, "latestProcessGraphRun")
	run, ok := active["run"].(map[string]any)
	if !ok || active["status"] != "ready" || run["completed"] != float64(0) || run["total"].(float64) <= 0 {
		t.Fatalf("full plan was unavailable before generation completed: %v", active)
	}
	total := int(run["total"].(float64))
	wantTotal, wantCalls := total, total
	if exhaustFirst {
		wantTotal++
		wantCalls += 2
	}
	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := call("token-a", latest, "latestProcessGraphRun")
		if status["status"] != "ready" {
			if final, ok := status["run"].(map[string]any); !ok || final["failed"] != float64(0) ||
				final["completed"] != float64(wantTotal) || final["total"] != float64(wantTotal) || int(generated.Load()) != wantCalls {
				t.Fatalf("expected %d settled batches after %d calls, got %d calls and result %v", wantTotal, wantCalls, generated.Load(), status)
			}
			t.Logf("profile with %d input tokens planned %d batches from %d probes and settled %d with zero failures", window, total, counted.Load(), wantTotal)
			return total
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("planned %d batches and did not finish", total)
	return 0
}
