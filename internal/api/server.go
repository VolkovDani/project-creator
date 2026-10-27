package api

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"github.com/VolkovDani/project-creator/graph"
	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	_ "github.com/mattn/go-sqlite3"
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
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	schema, err := graphql.ParseSchema(graph.Schema, &graph.Resolver{DB: db}, graphql.UseFieldResolvers(), graphql.MaxDepth(8), graphql.MaxParallelism(10))
	if err != nil {
		db.Close()
		return nil, err
	}
	h := &relay.Handler{Schema: schema}
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
		h.ServeHTTP(w, r.WithContext(graph.WithBudget(graph.WithWorkspace(r.Context(), ws))))
	})}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.h.ServeHTTP(w, r) }
func (s *Server) Close() error                                     { return s.db.Close() }

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return errors.New("database schema version is newer than supported")
	}
	if version < 1 {
		if _, err = tx.Exec(`CREATE TABLE projects(id TEXT PRIMARY KEY, workspace TEXT NOT NULL, name TEXT NOT NULL, goal TEXT NOT NULL, description TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE INDEX projects_workspace ON projects(workspace); INSERT INTO schema_migrations(version) VALUES(1)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
