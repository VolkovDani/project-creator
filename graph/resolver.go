package graph

import (
	"context"
	"database/sql"
)

type workspaceKey struct{}

func WithWorkspace(ctx context.Context, workspace string) context.Context {
	return context.WithValue(ctx, workspaceKey{}, workspace)
}
func workspace(ctx context.Context) string { v, _ := ctx.Value(workspaceKey{}).(string); return v }

type Resolver struct{ DB *sql.DB }
