package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func goodVector() []float32 { v := make([]float32, 1024); v[0] = 1; return v }
func TestOllamaPinnedModelAndBatch(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.URL.Path == "/api/tags" {
			json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": "bge-m3:567m", "digest": testDigest}}})
			return
		}
		var in struct {
			Model    string   `json:"model"`
			Input    []string `json:"input"`
			Truncate *bool    `json:"truncate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || in.Model != "bge-m3:567m" || len(in.Input) != 2 || in.Truncate == nil || *in.Truncate {
			t.Errorf("bad request %+v", in)
		}
		json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{goodVector(), goodVector()}})
	}))
	defer server.Close()
	provider, err := NewOllama(server.URL, "bge-m3:567m", testDigest)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := provider.Embed(context.Background(), []string{"Первое", "Second"})
	if err != nil || len(vectors) != 2 {
		t.Fatalf("%v %d", err, len(vectors))
	}
	if strings.Join(calls, ",") != "/api/tags,/api/embed,/api/tags" {
		t.Fatal(calls)
	}
	other, _ := NewOllama(server.URL, "bge-m3:567m", "sha256:"+strings.Repeat("b", 64))
	if other.Profile() == provider.Profile() || provider.Profile() == "" {
		t.Fatal("profile must include digest")
	}
}
func TestOllamaRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewOllama("http://ollama:11434", "bge-m3:567m", testDigest); err != nil {
		t.Fatalf("explicit Compose runtime rejected: %v", err)
	}
	for _, url := range []string{"https://example.com", "http://0.0.0.0:11434", "http://user:password@127.0.0.1:11434", "http://127.0.0.1:11434/path", "file:///tmp/test"} {
		if _, err := NewOllama(url, "bge-m3:567m", testDigest); err == nil {
			t.Errorf("accepted %q", url)
		}
	}
	if _, err := NewOllama("http://127.0.0.1:11434", "bge-m3:567m", "latest"); err == nil {
		t.Fatal("mutable digest accepted")
	}
}
func TestOllamaRejectsBadOutputAndDigestChanges(t *testing.T) {
	for _, scenario := range []string{"missing-model", "changed-after", "wrong-dimension", "wrong-count", "zero-vector", "nonfinite", "too-large", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			tags := 0
			embedCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					tags++
					models := []any{map[string]any{"name": "bge-m3:567m", "digest": testDigest}}
					if scenario == "missing-model" {
						models = nil
					}
					if scenario == "changed-after" && tags > 1 {
						models = []any{map[string]any{"name": "bge-m3:567m", "digest": "changed"}}
					}
					json.NewEncoder(w).Encode(map[string]any{"models": models})
					return
				}
				embedCalls++
				switch scenario {
				case "wrong-dimension":
					w.Write([]byte(`{"embeddings":[[1]]}`))
				case "wrong-count":
					w.Write([]byte(`{"embeddings":[]}`))
				case "zero-vector":
					json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{make([]float32, 1024)}})
				case "nonfinite":
					w.Write([]byte(`{"embeddings":[[1e1000]]}`))
				case "too-large":
					w.Write([]byte(strings.Repeat(" ", 8*1024*1024+1)))
				case "redirect":
					http.Redirect(w, r, "https://example.com", 302)
				default:
					json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{goodVector()}})
				}
			}))
			defer server.Close()
			p, err := NewOllama(server.URL, "bge-m3:567m", testDigest)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Embed(context.Background(), []string{"hello"}); err == nil {
				t.Fatal("bad provider result accepted")
			}
			if scenario == "missing-model" && embedCalls != 0 {
				t.Fatal("sent text before checking model")
			}
		})
	}
}
func TestOllamaContextAndInputLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	p, _ := NewOllama(server.URL, "bge-m3:567m", testDigest)
	if _, err := p.Embed(context.Background(), make([]string, 33)); err == nil {
		t.Fatal("oversized batch accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Embed(ctx, []string{"hello"}); err == nil {
		t.Fatal("context timeout ignored")
	}
}
func TestRealOllamaSmoke(t *testing.T) {
	if os.Getenv("OLLAMA_SMOKE") != "1" {
		t.Skip("opt in with OLLAMA_SMOKE=1 and OLLAMA_URL/MODEL/DIGEST")
	}
	p, err := NewOllama(os.Getenv("OLLAMA_URL"), os.Getenv("OLLAMA_MODEL"), os.Getenv("OLLAMA_DIGEST"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	v, err := p.Embed(ctx, []string{"Пользователь может восстановить пароль.", "The user can reset their password."})
	if err != nil || len(v) != 2 {
		t.Fatalf("smoke embeddings: %v", err)
	}
}
