package main

import (
	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/summary"
)

func configuredSummarizer(getenv func(string) string) (summary.Generator, error) {
	if getenv("PROJECT_CREATOR_GENERATOR_RUNTIME") == "openai-compatible" {
		return analysis.NewSummaryOpenAICompatible(getenv("PROJECT_CREATOR_GENERATOR_URL"), getenv("PROJECT_CREATOR_GENERATOR_MODEL"), getenv("PROJECT_CREATOR_GENERATOR_API_KEY"), getenv("PROJECT_CREATOR_SUMMARY_LANGUAGES"))
	}
	model, digest := getenv("PROJECT_CREATOR_SUMMARY_MODEL"), getenv("PROJECT_CREATOR_SUMMARY_DIGEST")
	if model == "" && digest == "" {
		return nil, nil
	}
	base := getenv("PROJECT_CREATOR_SUMMARY_URL")
	if base == "" {
		base = getenv("PROJECT_CREATOR_OLLAMA_URL")
	}
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	return analysis.NewSummaryOllamaWithLanguages(base, model, digest, getenv("PROJECT_CREATOR_SUMMARY_LANGUAGES"))
}
