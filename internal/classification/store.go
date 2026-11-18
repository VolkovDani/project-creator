// Package classification keeps manual decisions and earlier suggestions by source revision.
package classification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/project"
)

var rules = contracts.ClassificationRules()

type Alternative struct {
	SemanticType       string
	RepresentationType string
	Score              *float64
}
type Entry struct {
	ID, Status, Semantic, Representation, Source, RulesVersion, Model, CreatedAt string
	Version, Revision                                                            int32
	Alternatives                                                                 []Alternative
}

const projection = `r.id, coalesce(c.text_version,r.version), coalesce(c.revision,0),
CASE WHEN c.text_version<>r.version THEN 'stale' ELSE coalesce(c.status,'unclassified') END,
coalesce(c.semantic_type,'unknown'),coalesce(c.representation_type,'unknown'),
coalesce(c.source,''),coalesce(c.rules_version,''),coalesce(c.model,''),coalesce(c.created_at::text,''),coalesce(c.alternatives,'[]')`
const latest = ` FROM requirements r LEFT JOIN LATERAL (
SELECT * FROM requirement_classifications WHERE workspace=r.workspace AND project_id=r.project_id AND requirement_id=r.id
ORDER BY revision DESC LIMIT 1) c ON true WHERE r.workspace=$1 AND r.project_id=$2 `

type scanner interface{ Scan(...any) error }

func scan(s scanner) (Entry, error) {
	var v Entry
	var alternatives []byte
	err := s.Scan(&v.ID, &v.Version, &v.Revision, &v.Status, &v.Semantic, &v.Representation, &v.Source, &v.RulesVersion, &v.Model, &v.CreatedAt, &alternatives)
	if err == nil {
		err = json.Unmarshal(alternatives, &v.Alternatives)
	}
	return v, err
}
func Get(ctx context.Context, db *sql.DB, w, p, id string) (*Entry, error) {
	v, err := scan(db.QueryRowContext(ctx, "SELECT "+projection+latest+"AND r.id=$3", w, p, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}

const stored = "requirement_id,text_version,revision,status,semantic_type,representation_type,source,rules_version,model,created_at::text,alternatives"

func History(ctx context.Context, db *sql.DB, w, p, id string, limit, offset int32) ([]Entry, error) {
	return collect(db.QueryContext(ctx, "SELECT "+stored+" FROM requirement_classifications WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 ORDER BY revision DESC LIMIT $4 OFFSET $5", w, p, id, limit, offset))
}
func Filter(ctx context.Context, db *sql.DB, w, p string, semantic, representation *string, limit, offset int32) ([]Entry, error) {
	return collect(db.QueryContext(ctx, "SELECT "+projection+latest+`
	AND ($3::text IS NULL OR coalesce(c.semantic_type,'unknown')=$3)
	AND ($4::text IS NULL OR coalesce(c.representation_type,'unknown')=$4)
	ORDER BY r.created_at,r.id LIMIT $5 OFFSET $6`, w, p, semantic, representation, limit, offset))
}
func collect(rows *sql.Rows, err error) ([]Entry, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Entry
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func invalid(field, code, key string) []project.FieldError {
	return []project.FieldError{{Path: []string{"input", field}, Code: code, MessageKey: key, Params: map[string]int{}}}
}

func Confirm(ctx context.Context, db *sql.DB, w, p string, decision Entry) (*Entry, []project.FieldError, error) {
	if !slices.Contains(rules.Semantic, decision.Semantic) {
		return nil, invalid("semanticType", "INVALID_VALUE", "classification.invalidType"), nil
	}
	if !slices.Contains(rules.Representation, decision.Representation) {
		return nil, invalid("representationType", "INVALID_VALUE", "classification.invalidType"), nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	var version, revision int32
	err = tx.QueryRowContext(ctx, "SELECT version FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE", w, p, decision.ID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) || err == nil && version != decision.Version {
		return nil, invalid("textVersion", "CONFLICT", "validation.conflict"), nil
	}
	if err != nil {
		return nil, nil, err
	}
	err = tx.QueryRowContext(ctx, "SELECT coalesce(max(revision),0) FROM requirement_classifications WHERE requirement_id=$1", decision.ID).Scan(&revision)
	if err != nil {
		return nil, nil, err
	}
	if revision != decision.Revision {
		return nil, invalid("revision", "CONFLICT", "validation.conflict"), nil
	}
	v, err := scan(tx.QueryRowContext(ctx, `INSERT INTO requirement_classifications(workspace,project_id,requirement_id,text_version,revision,status,source,rules_version,semantic_type,representation_type)
	VALUES($1,$2,$3,$4,$5,'confirmed','user',$6,$7,$8) RETURNING `+stored, w, p, decision.ID, version, revision+1, rules.Version, decision.Semantic, decision.Representation))
	if err != nil {
		return nil, nil, err
	}
	return &v, nil, tx.Commit()
}
