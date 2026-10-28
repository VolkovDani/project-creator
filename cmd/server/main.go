package main

import (
	"github.com/VolkovDani/project-creator/internal/api"
	"github.com/VolkovDani/project-creator/internal/rag"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	token := os.Getenv("PROJECT_CREATOR_TOKEN")
	if len(token) < 32 {
		log.Fatal("PROJECT_CREATOR_TOKEN must have at least 32 characters")
	}
	path := os.Getenv("PROJECT_CREATOR_DATABASE_URL")
	if path == "" {
		log.Fatal("PROJECT_CREATOR_DATABASE_URL is required")
	}
	addr := os.Getenv("PROJECT_CREATOR_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if strings.TrimSpace(token) != token {
		log.Fatal("token must not contain edge whitespace")
	}
	var embedder rag.Embedder
	model := os.Getenv("PROJECT_CREATOR_EMBEDDING_MODEL")
	digest := os.Getenv("PROJECT_CREATOR_EMBEDDING_DIGEST")
	if model != "" || digest != "" {
		base := os.Getenv("PROJECT_CREATOR_OLLAMA_URL")
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		provider, err := rag.NewOllama(base, model, digest)
		if err != nil {
			log.Fatal("invalid local embedding configuration: ", err)
		}
		embedder = provider
	}
	s, err := api.NewWithEmbedder(path, map[string]string{token: "default"}, embedder)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	log.Fatal(http.ListenAndServe(addr, s))
}
