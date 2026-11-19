package main

import (
	"strings"
	"testing"
)

func TestIndependentCompatibleRuntimes(t *testing.T) {
	for _, generation := range []string{"openai-compatible", "ollama"} {
		for _, embedding := range []string{"openai-compatible", "ollama"} {
			env := map[string]string{"PROJECT_CREATOR_GENERATOR_RUNTIME": generation, "PROJECT_CREATOR_EMBEDDING_RUNTIME": embedding}
			for role, runtime := range map[string]string{"GENERATOR": generation, "EMBEDDING": embedding} {
				prefix := "PROJECT_CREATOR_" + role + "_"
				env[prefix+"MODEL"] = "model"
				if runtime == "openai-compatible" {
					env[prefix+"URL"] = "https://example.invalid/v1"
				} else {
					env[prefix+"DIGEST"] = strings.Repeat("a", 64)
				}
			}
			get := func(k string) string { return env[k] }
			if g, err := configuredGenerator(get); err != nil || g == nil {
				t.Fatal(generation, err)
			}
			if e, err := configuredEmbedder(get); err != nil || e == nil {
				t.Fatal(embedding, err)
			}
			if s, err := configuredSummarizer(get); err != nil || (s != nil) != (generation == "openai-compatible") {
				t.Fatal(generation, err)
			}
		}
	}
}
