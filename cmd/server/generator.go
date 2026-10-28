package main

import (
	"errors"
	"github.com/VolkovDani/project-creator/internal/analysis"
)

// A failed or unknown explicitly selected runtime must never become Ollama or
// an external provider implicitly. Empty default configuration disables analysis.
func configuredGenerator(getenv func(string) string) (analysis.Generator, error) {
	runtime := getenv("PROJECT_CREATOR_GENERATOR_RUNTIME")
	switch runtime {
	case "openvino-npu":
		return analysis.NewOpenVINO(getenv("PROJECT_CREATOR_NPU_SOCKET"), getenv("PROJECT_CREATOR_NPU_PROFILE"))
	case "", "ollama":
		model, digest := getenv("PROJECT_CREATOR_GENERATOR_MODEL"), getenv("PROJECT_CREATOR_GENERATOR_DIGEST")
		if model == "" && digest == "" {
			return nil, nil
		}
		base := getenv("PROJECT_CREATOR_OLLAMA_URL")
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return analysis.NewOllama(base, model, digest)
	default:
		return nil, errors.New("unknown PROJECT_CREATOR_GENERATOR_RUNTIME; expected ollama or openvino-npu")
	}
}
