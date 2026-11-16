package main

import (
	"strings"
	"testing"
)

func TestGeneratorConfigurationIsExplicitWithoutFallback(t *testing.T) {
	for _, tc := range []struct {
		name             string
		env              map[string]string
		wantErr, wantNil bool
	}{
		{"disabled", nil, false, true},
		{"unknown", map[string]string{"PROJECT_CREATOR_GENERATOR_RUNTIME": "auto"}, true, false},
		{"npu missing profile", map[string]string{"PROJECT_CREATOR_GENERATOR_RUNTIME": "openvino-npu", "PROJECT_CREATOR_NPU_SOCKET": "/run/npu/runtime.sock"}, true, false},
		{"npu", map[string]string{"PROJECT_CREATOR_GENERATOR_RUNTIME": "openvino-npu", "PROJECT_CREATOR_NPU_SOCKET": "/run/npu/runtime.sock", "PROJECT_CREATOR_NPU_PROFILE": strings.Repeat("a", 64)}, false, false},
		{"ollama incomplete", map[string]string{"PROJECT_CREATOR_GENERATOR_MODEL": "qwen2.5:7b"}, true, false},
		{"ollama", map[string]string{"PROJECT_CREATOR_GENERATOR_MODEL": "qwen2.5:7b", "PROJECT_CREATOR_GENERATOR_DIGEST": strings.Repeat("a", 64)}, false, false},
		{"ollama dedicated URL", map[string]string{"PROJECT_CREATOR_GENERATOR_MODEL": "qwen2.5:7b", "PROJECT_CREATOR_GENERATOR_DIGEST": strings.Repeat("a", 64), "PROJECT_CREATOR_GENERATOR_URL": "http://ollama-generator:11434"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generator, err := configuredGenerator(func(key string) string { return tc.env[key] })
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if err == nil && (generator == nil) != tc.wantNil {
				t.Fatalf("generator=%v", generator)
			}
		})
	}
}
