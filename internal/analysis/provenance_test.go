package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratorProvenancePinsEffectiveRequest(t *testing.T) {
	ollama, err := NewOllama("http://localhost:11434", "qwen2.5:7b", pinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	npu, err := NewOpenVINO("/tmp/test-npu.sock", strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, generator := range []Generator{ollama, npu} {
		raw := GeneratorProvenance(generator)
		var profile map[string]any
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(raw))
		if generator.Profile() != hex.EncodeToString(sum[:]) {
			t.Fatal("profile key does not hash saved provenance")
		}
		if profile["explanationAlphabet"] != "ru,en" || profile["alphabetRetries"] != float64(1) || profile["alphabetRetryInstruction"] != alphabetRetryInstruction {
			t.Fatal("missing pair explanation alphabet or retry policy")
		}
		if profile["systemPrompt"] != systemPrompt {
			t.Fatal("missing exact prompt")
		}
		schema, _ := json.Marshal(findingSchema())
		var expectedSchema any
		json.Unmarshal(schema, &expectedSchema)
		if !reflect.DeepEqual(profile["responseSchema"], expectedSchema) {
			t.Fatal("missing exact response schema")
		}
		if profile["adapter"] == "ollama-chat-v1" {
			if profile["model"] != "qwen2.5:7b" || profile["digest"] != pinnedDigest {
				t.Fatal("missing model identity")
			}
			params := profile["parameters"].(map[string]any)
			if params["temperature"] != float64(0) || params["num_ctx"] != float64(32768) || params["num_predict"] != float64(1024) {
				t.Fatal("missing inference parameters")
			}
		} else if profile["adapter"] == "openvino-npu-unix-v1" {
			if profile["runtimeProfile"] != strings.Repeat("b", 64) || profile["normalization"] != "source-aliases-v2:text-order" {
				t.Fatal("missing pinned NPU identity")
			}
		} else {
			t.Fatal("missing adapter identity")
		}
	}
	otherEndpoint, _ := NewOllama("http://localhost:11435", "qwen2.5:7b", pinnedDigest)
	if ollama.Profile() != otherEndpoint.Profile() {
		t.Fatal("transport location changes result identity")
	}
}

func TestProfileChangesWithEverySemanticComponent(t *testing.T) {
	base := generationProvenance{Adapter: "ollama-chat-v1", Model: "model", Digest: pinnedDigest, SystemPrompt: systemPrompt, ResponseSchema: findingSchema(), Parameters: ollamaParameters()}
	_, original := encodeGenerationProvenance(base)
	changes := []func(*generationProvenance){
		func(p *generationProvenance) { p.Adapter += "2" },
		func(p *generationProvenance) { p.Model += "2" },
		func(p *generationProvenance) { p.Digest = strings.Repeat("b", 64) },
		func(p *generationProvenance) { p.SystemPrompt += "updated" },
		func(p *generationProvenance) { p.ResponseSchema = map[string]any{"type": "string"} },
		func(p *generationProvenance) { p.Parameters = map[string]any{"temperature": 1} },
		func(p *generationProvenance) { p.RuntimeProfile = strings.Repeat("c", 64) },
		func(p *generationProvenance) { p.Normalization = "new" },
	}
	for _, change := range changes {
		modified := base
		change(&modified)
		_, hash := encodeGenerationProvenance(modified)
		if hash == original {
			t.Fatal("semantic change reused profile")
		}
	}
}
