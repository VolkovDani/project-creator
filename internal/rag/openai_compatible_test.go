package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestOpenAICompatibleEmbeddings(t *testing.T) {
	texts := []string{"Первое", "Second"}
	for _, requested := range []bool{false, true} {
		constructor := NewOpenAICompatible
		name := "native dimensions"
		if requested {
			constructor = NewOpenAICompatibleWithDimensions
			name = "request dimensions"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					Model      string   `json:"model"`
					Input      []string `json:"input"`
					Dimensions *int     `json:"dimensions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" || in.Model != "test-model" || !slices.Equal(in.Input, texts) {
					t.Errorf("unexpected %s %s, payload %+v", r.Method, r.URL.Path, in)
				}
				if (in.Dimensions != nil) != requested || (in.Dimensions != nil && *in.Dimensions != Dimensions) {
					t.Errorf("unexpected dimensions %v", in.Dimensions)
				}
				first, second := goodVector(), goodVector()
				second[0] = 2
				json.NewEncoder(w).Encode(map[string]any{"data": []any{
					map[string]any{"index": 1, "embedding": second},
					map[string]any{"index": 0, "embedding": first},
				}})
			}))
			defer server.Close()
			p, err := constructor(server.URL+"/v1", "test-model", "")
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Embed(context.Background(), texts)
			if err != nil || len(got) != len(texts) || got[0][0] != 1 || got[1][0] != 2 {
				t.Fatalf("incorrectly associated vectors, result count %d, error %v", len(got), err)
			}
		})
	}
}

func TestOpenAICompatibleEmbeddingProfile(t *testing.T) {
	profile := func(base, model, key string, dimensions bool) string {
		t.Helper()
		constructor := NewOpenAICompatible
		if dimensions {
			constructor = NewOpenAICompatibleWithDimensions
		}
		p, err := constructor(base, model, key)
		if err != nil {
			t.Fatal(err)
		}
		return p.Profile()
	}
	const base = "https://runtime.example/v1"
	initial := profile(base, "m1", "old-key", false)
	if len(initial) != 64 || profile(base, "m1", "new-key", false) != initial {
		t.Fatal("invalid or credential-dependent embedding profile")
	}
	for _, other := range []string{
		profile(base, "m2", "old-key", false),
		profile("https://second.example/v1", "m1", "old-key", false),
		profile(base, "m1", "old-key", true),
	} {
		if other == initial {
			t.Fatal("different embedding configurations share a profile")
		}
	}
}

func TestOpenAICompatibleRejectsInvalidEmbeddingResults(t *testing.T) {
	vector := goodVector()
	nullComponent := make([]any, Dimensions)
	nullComponent[0] = 1
	entry := func(index, vector any) any { return map[string]any{"index": index, "embedding": vector} }
	for name, data := range map[string][]any{
		"wrong count":      {entry(0, vector)},
		"duplicate index":  {entry(0, vector), entry(0, vector)},
		"negative index":   {entry(-1, vector), entry(0, vector)},
		"out of range":     {entry(0, vector), entry(2, vector)},
		"missing index":    {map[string]any{"embedding": vector}, entry(1, vector)},
		"null index":       {entry(nil, vector), entry(1, vector)},
		"wrong dimensions": {entry(0, []int{1}), entry(1, vector)},
		"all zero":         {entry(0, make([]float32, Dimensions)), entry(1, vector)},
		"null component":   {entry(0, nullComponent), entry(1, vector)},
		"overflow":         {entry(0, json.RawMessage("[1e1000,"+strings.Repeat("0,", Dimensions-2)+"0]")), entry(1, vector)},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"data": data})
			}))
			defer server.Close()
			p, err := NewOpenAICompatible(server.URL+"/v1", "test-model", "")
			if err != nil {
				t.Fatal(err)
			}
			if result, err := p.Embed(context.Background(), []string{"first", "second"}); err == nil || result != nil {
				t.Fatal("accepted invalid embedding response")
			}
		})
	}
}

func TestOpenAICompatibleValidatesEmbeddingInputsBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("sent invalid input to embedding provider")
	}))
	defer server.Close()
	p, err := NewOpenAICompatible(server.URL+"/v1", "test-model", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, texts := range [][]string{nil, make([]string, 33), {"ok", "  "}, {string([]byte{0xff})}, {strings.Repeat("я", 10001)}} {
		if _, err := p.Embed(context.Background(), texts); err == nil {
			t.Error("accepted invalid input")
		}
	}
}
