package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/VolkovDani/project-creator/graph"
	_ "github.com/mattn/go-sqlite3"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"net/http"
	"strings"
)

type Server struct {
	db *sql.DB
	h  http.Handler
}

func New(path string, tokens map[string]string) (*Server, error) {
	if len(tokens) == 0 {
		return nil, errors.New("at least one workspace token is required")
	}
	for token, workspace := range tokens {
		if len(token) < 7 || workspace == "" {
			return nil, errors.New("invalid workspace token")
		}
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY); CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY, workspace TEXT NOT NULL, name TEXT NOT NULL, goal TEXT NOT NULL, description TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE INDEX IF NOT EXISTS projects_workspace ON projects(workspace); INSERT OR IGNORE INTO schema_migrations(version) VALUES(1);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	h := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{DB: db}}))
	h.AddTransport(transport.POST{})
	h.Use(extension.FixedComplexityLimit(200))
	h.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		e := graphql.DefaultErrorPresenter(ctx, err)
		if e.Extensions == nil {
			e.Message = "Request failed"
		}
		return e
	})
	return &Server{db: db, h: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		h.ServeHTTP(w, r.WithContext(graph.WithWorkspace(r.Context(), ws)))
	})}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.h.ServeHTTP(w, r) }
func (s *Server) Close() error                                     { return s.db.Close() }
