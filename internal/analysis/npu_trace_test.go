package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNPUCallLogContainsOnlyTechnicalCorrelation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	trace := npuCall{RunID: strings.Repeat("a", 32), PairID: strings.Repeat("b", 32) + ":1", CallID: strings.Repeat("c", 32)}
	writeNPUCall(path, "compare_started", trace)
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var record map[string]string
	if e = json.Unmarshal(data, &record); e != nil {
		t.Fatal(e)
	}
	if record["runId"] != trace.RunID || record["pairId"] != trace.PairID || record["callId"] != trace.CallID || len(record) != 5 {
		t.Fatalf("unexpected fields %s", data)
	}
	writeNPUCall(path, "private text", npuCall{CallID: "private text"})
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("unsafe correlation logged")
	}
}
