package main

import (
	"github.com/VolkovDani/project-creator/internal/analysis"
)

func configuredClassifier(getenv func(string) string) (analysis.Classifier, error) {
	model := getenv("PROJECT_CREATOR_LAYA_MODEL")
	digest := getenv("PROJECT_CREATOR_LAYA_DIGEST")
	if model == "" && digest == "" {
		return nil, nil
	}
	base := getenv("PROJECT_CREATOR_LAYA_URL")
	if base == "" {
		base = getenv("PROJECT_CREATOR_OLLAMA_URL")
	}
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	return analysis.NewLaya(base, model, digest)
}
