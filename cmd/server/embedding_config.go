package main

import (
	"errors"

	"github.com/VolkovDani/project-creator/internal/rag"
)

func configuredEmbedder(getenv func(string) string) (rag.Embedder, error) {
	model := getenv("PROJECT_CREATOR_EMBEDDING_MODEL")
	base := getenv("PROJECT_CREATOR_EMBEDDING_URL")
	switch getenv("PROJECT_CREATOR_EMBEDDING_RUNTIME") {
	case "openai-compatible":
		key := getenv("PROJECT_CREATOR_EMBEDDING_API_KEY")
		switch getenv("PROJECT_CREATOR_EMBEDDING_DIMENSIONS") {
		case "":
			return rag.NewOpenAICompatible(base, model, key)
		case "1024":
			return rag.NewOpenAICompatibleWithDimensions(base, model, key)
		default:
			return nil, errors.New("configured embedding dimensions must be 1024")
		}
	case "", "ollama":
		digest := getenv("PROJECT_CREATOR_EMBEDDING_DIGEST")
		if model == "" && digest == "" {
			return nil, nil
		}
		if base == "" {
			base = getenv("PROJECT_CREATOR_OLLAMA_URL")
		}
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return rag.NewOllama(base, model, digest)
	default:
		return nil, errors.New("unknown PROJECT_CREATOR_EMBEDDING_RUNTIME")
	}
}
