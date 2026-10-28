package main

import (
	"context"
	"github.com/VolkovDani/project-creator/internal/api"
	"github.com/VolkovDani/project-creator/internal/rag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := healthcheck(ctx, "http://127.0.0.1:8080/graphql", os.Getenv("PROJECT_CREATOR_TOKEN")); err != nil {
			log.Fatal(err)
		}
		return
	}
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
	generator, err := configuredGenerator(os.Getenv)
	if err != nil {
		log.Fatal("invalid local analysis configuration: ", err)
	}
	s, err := api.NewWithProviders(path, map[string]string{token: "default"}, embedder, generator)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	log.Fatal(http.ListenAndServe(addr, s))
}
