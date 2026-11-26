package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Give both roles separate upstream credentials. Project Creator itself uses neither.
func startTestModelManager(t *testing.T, binary, upstream string) (base, chat, embedding string) {
	t.Helper()
	profiles := []map[string]string{
		{"name": "chat", "provider": "openai-compatible", "base_url": upstream, "role": "chat", "model": "chat", "revision": "test-1", "key_env": "GEN_KEY"},
		{"name": "embed", "provider": "openai-compatible", "base_url": upstream, "role": "embeddings", "model": "embed", "revision": "test-1", "key_env": "EMBED_KEY"},
	}
	base, models := startTestManager(t, binary, profiles, []string{"GEN_KEY=generation-key", "EMBED_KEY=embedding-key"})
	for _, model := range models {
		switch model.Role {
		case "chat":
			chat = model.ID
		case "embeddings":
			embedding = model.ID
		}
	}
	if chat == "" || embedding == "" || chat == embedding {
		t.Fatalf("unexpected inventory: %+v", models)
	}
	return base, chat, embedding
}

type testManagerModel struct{ ID, Role string }

func startTestManager(t *testing.T, binary string, profiles []map[string]string, env []string) (string, []testManagerModel) {
	t.Helper()
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
	cmd.Env = env
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	base := "http://" + address + "/v1"
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := client.Get(base + "/models")
		if err != nil {
			time.Sleep(30 * time.Millisecond)
			continue
		}
		var inventory struct{ Data []testManagerModel }
		err = json.NewDecoder(res.Body).Decode(&inventory)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("inventory returned %d: %v", res.StatusCode, err)
		}
		return base, inventory.Data
	}
	t.Fatalf("model manager did not start on %s", address)
	return "", nil
}
