package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

type npuCallKey struct{}
type npuCall struct{ RunID, PairID, CallID string }

var callIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var pairIDPattern = regexp.MustCompile(`^[0-9a-f]{32}:[0-9]{1,12}$`)
var npuLogMu sync.Mutex

func npuLogPath() string {
	current, err := os.Getwd()
	if err != nil {
		return ""
	}
	root := current
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			root = current
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return filepath.Join(root, "logs", "npu", "backend-lifecycle.jsonl")
}

func writeNPUCall(path, event string, call npuCall) {
	if path == "" || !callIDPattern.MatchString(call.CallID) {
		return
	}
	switch event {
	case "compare_started", "compare_returned":
	default:
		return
	}
	record := map[string]string{"time": time.Now().UTC().Format(time.RFC3339Nano), "event": event, "callId": call.CallID}
	if callIDPattern.MatchString(call.RunID) {
		record["runId"] = call.RunID
	}
	if callIDPattern.MatchString(call.PairID) || pairIDPattern.MatchString(call.PairID) {
		record["pairId"] = call.PairID
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	npuLogMu.Lock()
	defer npuLogMu.Unlock()
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(raw, '\n'))
}
