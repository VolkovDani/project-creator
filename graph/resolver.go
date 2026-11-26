package graph

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/explanation"
	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/processgraph"
	"github.com/VolkovDani/project-creator/internal/project"
	"github.com/VolkovDani/project-creator/internal/proposal"
	"github.com/VolkovDani/project-creator/internal/rag"
	"github.com/VolkovDani/project-creator/internal/summary"
	graphql "github.com/graph-gophers/graphql-go"
	"sync/atomic"
	"time"
)

type workspaceKey struct{}
type budgetKey struct{}

func WithBudget(ctx context.Context) context.Context {
	return context.WithValue(ctx, budgetKey{}, new(atomic.Int64))
}
func consume(ctx context.Context) error {
	budget, ok := ctx.Value(budgetKey{}).(*atomic.Int64)
	if !ok || budget.Add(1) > 100 {
		return errors.New("query operation limit exceeded")
	}
	return nil
}

func WithWorkspace(ctx context.Context, workspace string) context.Context {
	return context.WithValue(ctx, workspaceKey{}, workspace)
}
func workspace(ctx context.Context) string { v, _ := ctx.Value(workspaceKey{}).(string); return v }

type Resolver struct {
	ProcessGraphRuns *processgraph.RunStore
	ProcessGraphs    processgraph.Generator
	Proposals        *proposal.Service
	Explanations     *explanation.Service
	Classifications  *classification.Service
	Definitions      glossary.DefinitionGenerator
	Summaries        *summary.Service
	DB               *sql.DB
	RAG              *rag.Service
	Analysis         *analysis.Service
}

func (r *Resolver) CreateProject(ctx context.Context, args CreateProjectArgs) (*CreateProjectPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	in, errs := project.Validate(project.Input{Name: args.Input.Name, Goal: args.Input.Goal, Description: args.Input.Description})
	p := &CreateProjectPayload{FieldErrors: []*FieldError{}}
	for _, e := range errs {
		params := []*ErrorParam{}
		for k, v := range e.Params {
			params = append(params, &ErrorParam{Key: k, Value: int32(v)})
		}
		p.FieldErrors = append(p.FieldErrors, &FieldError{Path: e.Path, Code: e.Code, MessageKey: e.MessageKey, Params: params})
	}
	if len(errs) > 0 {
		return p, nil
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := &Project{ID: graphql.ID(hex.EncodeToString(id)), Name: in.Name, Goal: in.Goal, Description: in.Description, CreatedAt: now, UpdatedAt: now}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO projects(id,workspace,name,goal,description,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, item.ID, workspace(ctx), item.Name, item.Goal, item.Description, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return nil, errors.New("storage failure")
	}
	p.Project = item
	return p, nil
}
func (r *Resolver) Project(ctx context.Context, args ProjectArgs) (*Project, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	p := &Project{}
	err := r.DB.QueryRowContext(ctx, `SELECT id,name,goal,description,created_at,updated_at FROM projects WHERE workspace=$1 AND id=$2`, workspace(ctx), args.ID).Scan(&p.ID, &p.Name, &p.Goal, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("storage failure")
	}
	return p, nil
}
func (r *Resolver) Projects(ctx context.Context, args ProjectsArgs) ([]*Project, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if args.Limit < 1 || args.Limit > 100 || args.Offset < 0 {
		return nil, errors.New("invalid pagination")
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT id,name,goal,description,created_at,updated_at FROM projects WHERE workspace=$1 ORDER BY created_at,id LIMIT $2 OFFSET $3`, workspace(ctx), args.Limit, args.Offset)
	if err != nil {
		return nil, errors.New("storage failure")
	}
	defer rows.Close()
	out := []*Project{}
	for rows.Next() {
		p := &Project{}
		if err = rows.Scan(&p.ID, &p.Name, &p.Goal, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, errors.New("storage failure")
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		return nil, errors.New("storage failure")
	}
	return out, nil
}
