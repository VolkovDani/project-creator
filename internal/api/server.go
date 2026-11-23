package api

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/graph"
	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/rag"
	"github.com/VolkovDani/project-creator/internal/storage"
	"github.com/VolkovDani/project-creator/internal/summary"
	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"io"
	"net/http"
	"sort"
	"strings"
)

type Server struct {
	classifications *classification.Service
	summaries       *summary.Service
	db              *sql.DB
	h               http.Handler
	analysis        *analysis.Service
	indexing        *rag.Service
}

func New(path string, tokens map[string]string) (*Server, error) {
	return NewWithEmbedder(path, tokens, nil)
}

func NewWithEmbedder(path string, tokens map[string]string, embedder rag.Embedder) (*Server, error) {
	return NewWithProviders(path, tokens, embedder, nil)
}
func NewWithProviders(path string, tokens map[string]string, embedder rag.Embedder, generator analysis.Generator) (*Server, error) {
	return NewWithSummaryProviders(path, tokens, embedder, generator, nil)
}
func NewWithSummaryProviders(path string, tokens map[string]string, embedder rag.Embedder, generator analysis.Generator, summarizer summary.Generator) (*Server, error) {
	return NewConfigured(path, tokens, Config{
		Embedder: embedder, Generator: generator, Summarizer: summarizer,
		DefaultAnalysisThreshold: contracts.AnalysisConstraints().DefaultThreshold,
	})
}

type Config struct {
	Classifier               classification.Classifier
	Embedder                 rag.Embedder
	Generator                analysis.Generator
	Summarizer               summary.Generator
	DefaultAnalysisThreshold float64
}

func NewConfigured(path string, tokens map[string]string, cfg Config) (*Server, error) {
	if !contracts.AnalysisConstraints().ValidThreshold(cfg.DefaultAnalysisThreshold) {
		return nil, errors.New("invalid analysis default threshold")
	}
	if len(tokens) == 0 {
		return nil, errors.New("at least one workspace token is required")
	}
	for token, workspace := range tokens {
		if len(token) < 7 || workspace == "" {
			return nil, errors.New("invalid workspace token")
		}
	}
	db, err := storage.Open(path)
	if err != nil {
		return nil, err
	}
	summaries := summary.New(db, cfg.Summarizer)
	workspaces := make([]string, 0, len(tokens))
	seen := map[string]bool{}
	for _, w := range tokens {
		if !seen[w] {
			workspaces = append(workspaces, w)
			seen[w] = true
		}
	}
	sort.Strings(workspaces)
	if err = summaries.Start(workspaces); err != nil {
		db.Close()
		return nil, errors.New("summary queue startup unavailable")
	}
	retriever := rag.New(db, cfg.Embedder)
	embeddingProfile := ""
	if cfg.Embedder != nil {
		embeddingProfile = cfg.Embedder.Profile()
	}
	analyzer := analysis.NewWithDefaultThreshold(db, retriever, cfg.Generator, embeddingProfile, cfg.DefaultAnalysisThreshold)
	generatorProfile := ""
	if cfg.Generator != nil {
		generatorProfile = analyzer.Profile()
	}
	classifications := classification.New(db, cfg.Classifier)
	definitions, _ := cfg.Summarizer.(glossary.DefinitionGenerator)
	schema, err := graphql.ParseSchema(graph.Schema, &graph.Resolver{DB: db, Classifications: classifications, RAG: retriever, Analysis: analyzer, Summaries: summaries, Definitions: definitions}, graphql.UseFieldResolvers(), graphql.MaxDepth(8), graphql.MaxParallelism(10))
	if err != nil {
		analyzer.Close()
		summaries.Close()
		db.Close()
		return nil, err
	}
	if err = retriever.StartQueue(workspaces); err != nil {
		analyzer.Close()
		summaries.Close()
		db.Close()
		return nil, errors.New("index queue startup unavailable")
	}
	if err = classifications.Start(workspaces); err != nil {
		classifications.Close()
		retriever.Close()
		analyzer.Close()
		summaries.Close()
		db.Close()
		return nil, errors.New("classification queue startup unavailable")
	}
	if err = announceStartup(db, tokens); err != nil {
		classifications.Close()
		retriever.Close()
		analyzer.Close()
		summaries.Close()
		db.Close()
		return nil, errors.New("startup cache invalidation unavailable")
	}
	h := &relay.Handler{Schema: schema}
	return &Server{db: db, classifications: classifications, analysis: analyzer, indexing: retriever, summaries: summaries, h: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" && r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		ws := ""
		for candidate, workspace := range tokens {
			if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
				ws = workspace
			}
		}
		if ws == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/events" {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			serveEvents(db, ws, w, r, generatorProfile, embeddingProfile)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		var request struct {
			Query string `json:"query"`
		}
		if err = json.Unmarshal(body, &request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err = validateQueryBudget(request.Query); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"message":"invalid or oversized query"}]}`))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		serveTaggedGraphQL(h, w, r.WithContext(graph.WithBudget(graph.WithWorkspace(r.Context(), ws))))
	})}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.h.ServeHTTP(w, r) }
func (s *Server) Close() error {
	s.classifications.Close()
	s.summaries.Close()
	s.analysis.Close()
	s.indexing.Close()
	return s.db.Close()
}
