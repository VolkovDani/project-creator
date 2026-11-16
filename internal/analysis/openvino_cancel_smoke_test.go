package analysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This probe intentionally terminates one real NPU inference. Run against an
// idle, explicitly chosen local runtime; ordinary CI never touches hardware.
func TestOpenVINORealNPUCancelThenGenerate(t *testing.T) {
	if os.Getenv("NPU_REAL_CANCEL_SMOKE") != "1" {
		t.Skip("explicit NPU_REAL_CANCEL_SMOKE=1 and idle local runtime required")
	}
	generator, err := NewOpenVINO(os.Getenv("PROJECT_CREATOR_NPU_SOCKET"), os.Getenv("PROJECT_CREATOR_NPU_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	callID, err := identifier()
	if err != nil {
		t.Fatal(err)
	}
	pair := Pair{Left: Source{ID: "left", Version: 1, Text: "При любом заказе оплата разрешена только картой."}, Right: Source{ID: "right", Version: 1, Text: "При любом заказе оплата разрешена только наличными."}}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), npuCallKey{}, npuCall{CallID: callID}), 120*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, e := generator.Compare(ctx, pair); result <- e }()
	path := filepath.Join(filepath.Dir(npuLogPath()), "runtime-lifecycle.jsonl")
	waitForNativeEvent(t, path, "generation_started", callID, 120*time.Second)
	started := time.Now()
	cancel()
	select {
	case e := <-result:
		if e == nil {
			t.Fatal("cancelled inference returned success")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("adapter cancellation exceeded bound")
	}
	waitForNativeEvent(t, path, "worker_stopped", callID, 15*time.Second)
	nextCtx, nextCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer nextCancel()
	finding, e := generator.Compare(nextCtx, pair)
	if e != nil {
		t.Fatal(e)
	}
	if !validFinding(pair, finding) {
		t.Fatal("invalid new finding after cancellation")
	}
	t.Logf("cancel-to-regeneration %.2fs, pinned profile %s; quality is evaluated separately", time.Since(started).Seconds(), generator.Profile())
}

func waitForNativeEvent(t *testing.T, path, event, callID string, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				var entry struct {
					Event  string `json:"event"`
					CallID string `json:"call_id"`
				}
				if json.Unmarshal([]byte(line), &entry) == nil && entry.Event == event && entry.CallID == callID {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("runtime event %s for call %s not observed in %s", event, callID, path)
}
