package recordclassifier

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the decision wire contract against a binary built in the manager repository.
func TestManagedDecisionManagerIntegration(t *testing.T) {
	binary := os.Getenv("MODELS_CONFIGURATOR_TEST_BIN")
	if binary == "" {
		t.Skip("set MODELS_CONFIGURATOR_TEST_BIN to run the cross-repository decision test")
	}
	var calls atomic.Int32
	mock := testOllamaDecision(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		mock.ServeHTTP(w, r)
	}))
	defer upstream.Close()

	base, id := startDecisionManager(t, binary, upstream.URL)
	managed, err := NewManaged(base, id, Model, Digest)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := New(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	want, err := direct.Classify(t.Context(), testManagedSource)
	if err != nil || len(want.Alternatives) != 17 {
		t.Fatalf("direct prediction %+v: %v", want, err)
	}
	got, err := managed.Classify(t.Context(), testManagedSource)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("managed prediction %+v: %v, want %+v", got, err, want)
	}

	before := calls.Load()
	got, err = managed.Classify(t.Context(), testManagedSource+" <mask>")
	if err != nil || got.Reason != "reserved_token" || len(got.Alternatives) != 0 || calls.Load() != before {
		t.Fatalf("reserved token reached upstream or lost its disposition: %+v: %v", got, err)
	}

	wrongDigest, err := NewManaged(base, id, Model, d3Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongDigest.Classify(t.Context(), testManagedSource); err == nil || calls.Load() != before {
		t.Fatal("wrong digest reached upstream or was accepted")
	}
}

func startDecisionManager(t *testing.T, binary, upstream string) (string, string) {
	t.Helper()
	profiles := []map[string]string{
		{"name": "decision-review", "provider": "ollama", "base_url": upstream, "role": "decision", "model": Model, "digest": Digest},
	}
	data, err := json.Marshal(map[string]any{"models": profiles})
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	port, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.Addr().String()
	if err := port.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cmd := exec.CommandContext(ctx, binary, "-config", config, "-listen", address)
	cmd.Env = []string{}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	base := "http://" + address + "/v1"
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{}}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := client.Get(base + "/models")
		if err != nil {
			time.Sleep(30 * time.Millisecond)
			continue
		}
		var inventory struct {
			Data []struct{ ID, Role, Model string }
		}
		err = json.NewDecoder(res.Body).Decode(&inventory)
		res.Body.Close()
		if err != nil || res.StatusCode != http.StatusOK || len(inventory.Data) != 1 || inventory.Data[0].Role != "decision" || inventory.Data[0].Model != Model {
			t.Fatalf("unexpected manager inventory: %+v HTTP %d: %v", inventory, res.StatusCode, err)
		}
		return base, inventory.Data[0].ID
	}
	t.Fatalf("manager did not start at %s", address)
	return "", ""
}
