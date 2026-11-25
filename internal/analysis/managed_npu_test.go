package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/VolkovDani/project-creator/internal/glossary"
)

func TestManagedNPUSummaryLanguageRepairKeepsOneProperty(t *testing.T) {
	const preview = "Хранить отчёт в СЭД 42 дня."
	for _, tc := range []struct {
		name                string
		firstSummary        string
		ignoreFieldCount    bool
		losesNumberOnRepair bool
		orderedRepairFails  bool
		wantErr             error
	}{
		{name: "wrong language", firstSummary: "Retain the report for 42 days."},
		{name: "language repair drops the number", firstSummary: "Retain the report for 42 days.", losesNumberOnRepair: true},
		{name: "optional numeric retry is unavailable", firstSummary: "Retain the report for 42 days.", losesNumberOnRepair: true, orderedRepairFails: true, wantErr: ErrSummaryNumbers},
		{name: "missing number", firstSummary: "Хранить отчёт в СЭД."},
		{name: "provider adds an empty key", firstSummary: "Retain the report for 42 days.", ignoreFieldCount: true, wantErr: ErrSummaryFormat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := summarySource(preview)
			hint := glossary.Hint{TermID: "term-7", Spelling: "СЭД", Definition: "Система электронного документооборота."}
			hints := []glossary.Hint{hint}
			hints[0].Version = 3
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req chatRequestFixture
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				calls++
				if calls == 3 && tc.orderedRepairFails {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				schema := req.Format.Schema.Schema
				content := fmt.Sprintf(`{"summary":%q}`, tc.firstSummary)
				if calls == 1 {
					if schema["minProperties"] != nil || schema["maxProperties"] != nil {
						t.Error("the initial schema must retain the existing generation profile")
					}
				} else {
					var input struct {
						Text     string
						Glossary []glossary.Hint `json:"confirmedGlossary"`
					}
					if len(req.Messages) != 2 ||
						json.Unmarshal([]byte(req.Messages[1]["content"]), &input) != nil ||
						input.Text != src.Text || len(input.Glossary) != 1 || input.Glossary[0] != hint {
						t.Error("the repair must restart with the original text and glossary")
					}
					if !strings.Contains(req.Messages[0]["content"], "В выжимке сохраните следующие числа и обозначения: 42.") {
						t.Error("the language repair must explicitly carry every source number")
					}
					properties, _ := schema["properties"].(map[string]any)
					summary, _ := properties["summary"].(map[string]any)
					pattern, ok := summary["pattern"].(string)
					if !ok {
						t.Error("missing corrective language constraint")
					} else if calls == 3 {
						constraint := regexp.MustCompile(pattern)
						if !constraint.MatchString(preview) || constraint.MatchString("Хранить отчёт в СЭД.") {
							t.Error("the correction must require the source number alongside Russian text")
						}
					}
					// The managed runtime may emit an empty extra key under the
					// corrective pattern unless the object has an explicit limit.
					content = `{"summary":"` + preview + `"}`
					if tc.losesNumberOnRepair && calls == 2 {
						content = `{"summary":"Хранить отчёт в СЭД."}`
					}
					if schema["minProperties"] != float64(1) || schema["maxProperties"] != float64(1) || tc.ignoreFieldCount {
						content = `{"summary":"` + preview + `","":""}`
					}
				}
				fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":%q}}]}`, content)
			}))
			defer server.Close()
			g, err := NewSummaryOpenAICompatible(server.URL, "openvino-npu.test."+strings.Repeat("a", 64), "", "ru")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(g.Provenance(), "maxProperties") ||
				!strings.Contains(g.ResultProvenance(), "managed-npu-language-repair-exactly-one-property") {
				t.Fatal("record the repair policy without changing the base generation profile")
			}
			want := preview
			if tc.wantErr != nil {
				want = ""
			}
			wantCalls := 2
			if tc.losesNumberOnRepair {
				wantCalls = 3
			}
			if got, err := g.SummarizeWithGlossary(context.Background(), src, hints); got != want || !errors.Is(err, tc.wantErr) || calls != wantCalls {
				t.Fatalf("summary=%q calls=%d err=%v", got, calls, err)
			}
		})
	}
}

func TestManagedNPUPairParity(t *testing.T) {
	id := "openvino-npu.A.1_test-2." + strings.Repeat("a", 64)
	a := Source{ID: "db:alpha", Version: 37, Text: "A \"field\": \"a,b\",\t\\u2028 <&>\n\b\f\r\x00 русский \u2028\u2029"}
	z := Source{ID: "db:omega", Version: 91, Text: "Z\\, :"}
	pre := &Preclassification{Decision: "related", Scores: map[string]float64{"related": .99999, "uncertain": .00001, "unrelated": 0}}
	wantUser := `{"preclassification": {"decision": "related", "scores": {"related": 0.99999, "uncertain": 1e-05, "unrelated": 0}}, "left": {"id": "left", "version": 1, "text": "A \"field\": \"a,b\",\t\\u2028 <&>\n\b\f\r\u0000 русский ` + "\u2028\u2029" + `"}, "right": {"id": "right", "version": 1, "text": "Z\\, :"}}`
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequestFixture
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		wantPrompt, wantSchema := systemPrompt, findingSchema()
		content := npuFindingJSON()
		retry := calls%2 == 1
		calls++
		if retry {
			wantPrompt += "\n" + npuAlphabetRetryInstruction
			wantSchema = npuAlphabetRetrySchema()
		} else {
			content = strings.Replace(content, "В одном", "中文", 1)
		}
		gotSchema, _ := json.Marshal(req.Format.Schema.Schema)
		expected, _ := json.Marshal(wantSchema)
		if req.Model != id || len(req.Messages) != 2 || req.Messages[0]["content"] != wantPrompt || req.Messages[1]["content"] != wantUser || string(expected) != string(gotSchema) {
			t.Errorf("managed NPU request differs: user=%q schema matches=%t", req.Messages[1]["content"], string(expected) == string(gotSchema))
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	defer server.Close()
	g, err := NewOpenAICompatible(server.URL+"/v1", id, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []Pair{{Left: z, Right: a, Preclassification: pre}, {Left: a, Right: z, Preclassification: pre}} {
		result, err := g.Compare(context.Background(), pair)
		if err != nil || !validFinding(pair, result) || result.Verdict != "collision" {
			t.Fatalf("finding=%+v err=%v", result, err)
		}
	}
	if calls != 4 {
		t.Fatalf("got %d calls", calls)
	}
	var provenance generationProvenance
	if err := json.Unmarshal([]byte(g.Provenance()), &provenance); err != nil || provenance.Adapter != "openai-compatible-managed-npu-pair-v1" || provenance.Normalization != "source-aliases-v2:text-order" || provenance.AlphabetRetryInstruction != npuAlphabetRetryInstruction || provenance.AlphabetRetrySchema == nil || provenance.Model != id {
		t.Fatal("missing managed NPU policy provenance", err)
	}
}

func TestManagedNPUPolicyRequiresCompleteModelID(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, tc := range []struct {
		id      string
		managed bool
	}{
		{"openvino-npu." + strings.Repeat("x", 80) + "." + digest, true},
		{"openvino-npu." + strings.Repeat("x", 81) + "." + digest, false},
		{"openvino-npu.." + digest, false}, {"OpenVINO-NPU.foo." + digest, false}, {"openvino-npu.-test." + digest, false}, {"openvino-npu.foo/bar." + digest, false}, {"openvino-npu.foo." + strings.ToUpper(digest), false}, {"openvino-npu.foo." + digest + "x", false}, {"openvino-npu.foo." + digest[:63], false}, {"ollama.foo." + digest, false},
	} {
		g, err := NewOpenAICompatible("http://localhost:8317/v1", tc.id, "")
		if err != nil {
			t.Fatal(err)
		}
		var spec generationProvenance
		if err := json.Unmarshal([]byte(g.Provenance()), &spec); err != nil {
			t.Fatal(err)
		}
		if (spec.Adapter == "openai-compatible-managed-npu-pair-v1") != tc.managed || (!tc.managed && spec.Adapter != "openai-compatible-pair-v1") {
			t.Errorf("%s policy = %s", tc.id, spec.Adapter)
		}
	}
}

func TestManagedNPURejectsForeignSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":%q}}]}`, findingJSON())
	}))
	defer server.Close()
	g, err := NewOpenAICompatible(server.URL, "openvino-npu.test."+strings.Repeat("a", 64), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := g.Compare(context.Background(), pairFixture()); err == nil || got != (Finding{}) {
		t.Fatalf("accepted original IDs from model: %+v", got)
	}
}

func TestManagedNPUErrorClassification(t *testing.T) {
	const private = "PRIVATE diagnostic"
	for _, tc := range []struct {
		status     int
		code, want string
	}{
		{502, "INVALID_OUTPUT", "INVALID_OUTPUT"},
		{409, "PROFILE_CHANGED", "PROFILE_CHANGED"},
		{504, "TIMEOUT", "TIMEOUT"},
		{503, "NPU_UNAVAILABLE", "NPU_UNAVAILABLE"},
		{503, "NPU_RECOVERY_FAILED", "NPU_RECOVERY_FAILED"},
		{503, "NPU_WORKER_EXIT", "NPU_WORKER_EXIT"},
		{401, "INVALID_OUTPUT", "GENERATION_FAILED"},
		{502, "untrusted", "GENERATION_FAILED"},
		{502, "oversized", "GENERATION_FAILED"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			code, message := tc.code, private
			if code == "oversized" {
				code = "INVALID_OUTPUT"
				message = strings.Repeat("x", 4096)
			}
			fmt.Fprintf(w, `{"error":{"code":%q,"message":%q}}`, code, message)
		}))
		for _, model := range []string{"openvino-npu.test." + strings.Repeat("a", 64), "another-model"} {
			g, err := NewOpenAICompatible(server.URL, model, "")
			if err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if model == "another-model" {
				want = "GENERATION_FAILED"
			}
			if _, code := checkedCompare(context.Background(), g, pairFixture()); code != want {
				t.Errorf("HTTP %d model %s returned %s, want %s", tc.status, model, code, want)
			}
		}
		server.Close()
	}
}

func TestManagedNPUInvalidOutputContinuesFullQueue(t *testing.T) {
	db, retriever, _ := analysisDB(t)
	ready(t, retriever)
	addThird(t, db, retriever)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 2 {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `{"error":{"code":"INVALID_OUTPUT","message":"model provider failed"}}`)
			return
		}
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":%q}}]}`, npuFindingJSON())
	}))
	defer server.Close()
	g, err := NewOpenAICompatible(server.URL, "openvino-npu.test."+strings.Repeat("a", 64), "")
	if err != nil {
		t.Fatal(err)
	}
	service := testService(t, db, retriever, g, "embed-a")
	run, err := service.startAndWait(t, context.Background(), "a", "p")
	if err != nil || run.ErrorCode != "INVALID_OUTPUT" || run.ProcessedPairs != 3 || run.CompletedPairs != 2 || run.FailedPairs != 1 || calls.Load() != 3 {
		t.Fatalf("completed=%d failed=%d processed=%d code=%s calls=%d err=%v", run.CompletedPairs, run.FailedPairs, run.ProcessedPairs, run.ErrorCode, calls.Load(), err)
	}
}
