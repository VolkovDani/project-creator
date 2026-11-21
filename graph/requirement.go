package graph

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/VolkovDani/project-creator/internal/project"
	"github.com/VolkovDani/project-creator/internal/requirement"
	graphql "github.com/graph-gophers/graphql-go"
)

func fieldErrors(in []project.FieldError) []*FieldError {
	out := make([]*FieldError, 0, len(in))
	for _, e := range in {
		p := []*ErrorParam{}
		for k, v := range e.Params {
			p = append(p, &ErrorParam{Key: k, Value: int32(v)})
		}
		out = append(out, &FieldError{Path: e.Path, Code: e.Code, MessageKey: e.MessageKey, Params: p})
	}
	return out
}
func (r *Resolver) Requirements(ctx context.Context, args RequirementsArgs) ([]*Requirement, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if args.Limit < 1 || args.Limit > 100 || args.Offset < 0 {
		return nil, errors.New("invalid pagination")
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT id,project_id,text,version,created_at,updated_at FROM active_requirements WHERE workspace=$1 AND project_id=$2 ORDER BY created_at,id LIMIT $3 OFFSET $4`, workspace(ctx), args.ProjectId, args.Limit, args.Offset)
	if err != nil {
		return nil, errors.New("storage failure")
	}
	defer rows.Close()
	out := []*Requirement{}
	for rows.Next() {
		p := &Requirement{}
		if err := rows.Scan(&p.ID, &p.ProjectId, &p.Text, &p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, errors.New("storage failure")
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		return nil, errors.New("storage failure")
	}
	return out, nil
}
func (r *Resolver) CreateRequirement(ctx context.Context, args CreateRequirementArgs) (*RequirementPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	value, validation := requirement.Validate(args.Input.Text)
	payload := &RequirementPayload{FieldErrors: fieldErrors(validation)}
	if len(validation) > 0 {
		return payload, nil
	}
	var exists bool
	if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE workspace=$1 AND id=$2)`, workspace(ctx), args.Input.ProjectId).Scan(&exists); err != nil {
		return nil, errors.New("storage failure")
	}
	if !exists {
		return payload, nil
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	p := &Requirement{ID: graphql.ID(hex.EncodeToString(id)), ProjectId: args.Input.ProjectId, Text: value, Version: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := r.DB.ExecContext(ctx, `INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, p.ID, workspace(ctx), p.ProjectId, p.Text, p.Version, now, now); err != nil {
		return nil, errors.New("storage failure")
	}
	payload.Requirement = p
	return payload, nil
}
func (r *Resolver) UpdateRequirement(ctx context.Context, args UpdateRequirementArgs) (*RequirementPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	value, validation := requirement.Validate(args.Input.Text)
	payload := &RequirementPayload{FieldErrors: fieldErrors(validation)}
	if len(validation) > 0 {
		return payload, nil
	}
	p := &Requirement{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := r.DB.QueryRowContext(ctx, `UPDATE requirements SET text=$1,version=version+1,updated_at=$2 WHERE id=$3 AND project_id=$4 AND workspace=$5 AND version=$6 AND archived_at IS NULL RETURNING id,project_id,text,version,created_at,updated_at`, value, now, args.Input.ID, args.Input.ProjectId, workspace(ctx), args.Input.Version).Scan(&p.ID, &p.ProjectId, &p.Text, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		payload.FieldErrors = append(payload.FieldErrors, &FieldError{Path: []string{"input", "text"}, Code: "CONFLICT", MessageKey: "validation.conflict", Params: []*ErrorParam{}})
		return payload, nil
	}
	if err != nil {
		return nil, errors.New("storage failure")
	}
	payload.Requirement = p
	return payload, nil
}

func (r *Resolver) ArchiveRequirement(ctx context.Context, args ArchiveRequirementArgs) (bool, error) {
	if err := consume(ctx); err != nil {
		return false, err
	}
	return r.changeArchivedRequirement(ctx, args.ProjectId, `UPDATE requirements SET archived_at=$5
		WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4 AND archived_at IS NULL`,
		workspace(ctx), args.ProjectId, args.ID, args.Version, time.Now().UTC().Format(time.RFC3339Nano))
}

func (r *Resolver) RestoreRequirement(ctx context.Context, args RestoreRequirementArgs) (bool, error) {
	if err := consume(ctx); err != nil {
		return false, err
	}
	return r.changeArchivedRequirement(ctx, args.ProjectId, `UPDATE requirements SET archived_at=NULL
		WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4 AND archived_at=$5`,
		workspace(ctx), args.ProjectId, args.ID, args.Version, args.ArchivedAt)
}

// Only successful scoped writes may contribute entity tags to the HTTP response.
func (r *Resolver) changeArchivedRequirement(ctx context.Context, projectID graphql.ID, statement string, values ...any) (bool, error) {
	result, err := r.DB.ExecContext(ctx, statement, values...)
	if err != nil {
		return false, errors.New("storage failure")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, errors.New("storage failure")
	}
	if count != 1 {
		return false, nil
	}
	addCacheDependency(ctx, "projects")
	addCacheDependency(ctx, "project:"+string(projectID))
	addCacheDependency(ctx, "project:"+string(projectID)+":requirements")
	return true, nil
}

func (r *Resolver) DeleteRequirement(ctx context.Context, args DeleteRequirementArgs) (*DeleteRequirementPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if args.Confirmed == nil || !*args.Confirmed || args.ArchivedAt == nil {
		return &DeleteRequirementPayload{Deleted: false}, nil
	}
	deleted, err := r.changeArchivedRequirement(ctx, args.ProjectId, `DELETE FROM requirements WHERE id=$1 AND project_id=$2 AND workspace=$3 AND version=$4 AND archived_at=$5`, args.ID, args.ProjectId, workspace(ctx), args.Version, args.ArchivedAt)
	if err != nil {
		return nil, errors.New("storage failure")
	}
	return &DeleteRequirementPayload{Deleted: deleted}, nil
}
