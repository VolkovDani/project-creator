package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/VolkovDani/project-creator/internal/api"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "mcp" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runMCP(ctx, os.Args[2:], os.Getenv); err != nil {
			log.Fatal(err)
		}
		return
	}
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
	embedder, err := configuredEmbedder(os.Getenv)
	if err != nil {
		log.Fatal("invalid embedding configuration: ", err)
	}
	generator, err := configuredGenerator(os.Getenv)
	if err != nil {
		log.Fatal("invalid analysis configuration: ", err)
	}
	summarizer, err := configuredSummarizer(os.Getenv)
	if err != nil {
		log.Fatal("invalid summary configuration: ", err)
	}
	classifier, err := configuredRecordClassifier(os.Getenv)
	if err != nil {
		log.Fatal("invalid record classification configuration: ", err)
	}
	threshold, err := configuredAnalysisDefaultThreshold(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	retentionDays, err := configuredAnalysisHistoryRetentionDays(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	s, err := api.NewConfigured(path, map[string]string{token: "default"}, api.Config{
		Embedder: embedder, Generator: generator, Summarizer: summarizer, Classifier: classifier,
		DefaultAnalysisThreshold: threshold, AnalysisHistoryRetentionDays: retentionDays,
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = serveHTTP(ctx, &http.Server{Addr: addr, Handler: s, BaseContext: func(net.Listener) context.Context { return ctx }})
	closeErr := s.Close()
	if err != nil {
		log.Fatal(err)
	}
	if closeErr != nil {
		log.Fatal(closeErr)
	}
}
