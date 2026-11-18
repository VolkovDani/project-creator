package graph

import (
	"context"
	"database/sql"
	"errors"
	"github.com/VolkovDani/project-creator/internal/glossary"
	graphql "github.com/graph-gophers/graphql-go"
	"strings"
)

func glossaryTerm(t glossary.Term) *GlossaryTerm {
	v := &GlossaryTerm{ID: graphql.ID(t.ID), Name: t.Name, Definition: t.Definition, Context: t.Context, Version: int32(t.Version), SourceId: graphql.ID(t.SourceID), SourceVersion: int32(t.SourceVersion), UpdatedAt: t.UpdatedAt, CreatedAt: t.CreatedAt}
	for _, a := range t.Variants {
		v.Variants = append(v.Variants, &GlossaryVariant{Text: a.Text, Kind: a.Kind})
	}
	return v
}
func (r *Resolver) GlossaryTerms(ctx context.Context, a GlossaryTermsArgs) ([]*GlossaryTerm, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if a.Limit < 1 || a.Limit > 100 || a.Offset < 0 {
		return nil, errors.New("invalid pagination")
	}
	terms, e := glossary.List(ctx, r.DB, workspace(ctx), string(a.ProjectId))
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	var out []*GlossaryTerm
	search := strings.ToLower(strings.TrimSpace(a.Search))
	skip := a.Offset
	for _, t := range terms {
		found := strings.Contains(strings.ToLower(t.Name), search)
		for _, v := range t.Variants {
			if strings.Contains(strings.ToLower(v.Text), search) {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		out = append(out, glossaryTerm(t))
		if len(out) == int(a.Limit) {
			break
		}
	}
	return out, nil
}
func (r *Resolver) SaveGlossaryTerm(ctx context.Context, a SaveGlossaryTermArgs) (*GlossaryPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	in := a.Input
	t := glossary.Term{Name: in.Name, Definition: in.Definition, Context: in.Context, Variants: []glossary.Variant{}}
	if in.ID != nil {
		t.ID = string(*in.ID)
	}
	if in.Version != nil {
		t.Version = int(*in.Version)
	}
	if in.SourceId != nil {
		t.SourceID = string(*in.SourceId)
	}
	if in.SourceVersion != nil {
		t.SourceVersion = int(*in.SourceVersion)
	}
	for _, v := range in.Variants {
		t.Variants = append(t.Variants, glossary.Variant{Text: v.Text, Kind: v.Kind})
	}
	saved, errs, e := glossary.Change(ctx, r.DB, workspace(ctx), string(in.ProjectId), t, false)
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	out := &GlossaryPayload{FieldErrors: fieldErrors(errs)}
	if len(errs) == 0 {
		out.Term = glossaryTerm(saved)
	}
	return out, nil
}
func (r *Resolver) DeleteGlossaryTerm(ctx context.Context, a DeleteGlossaryTermArgs) (*DeleteGlossaryPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	_, errs, e := glossary.Change(ctx, r.DB, workspace(ctx), string(a.ProjectId), glossary.Term{ID: string(a.ID), Version: int(a.Version)}, true)
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	return &DeleteGlossaryPayload{Deleted: len(errs) == 0}, nil
}
func (r *Resolver) RequirementGlossary(ctx context.Context, a RequirementGlossaryArgs) (*RequirementGlossary, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	w, p := workspace(ctx), string(a.ProjectId)
	tx, e := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	defer tx.Rollback()
	var source string
	e = tx.QueryRowContext(ctx, `SELECT text FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4`, w, p, a.RequirementId, a.Version).Scan(&source)
	if e == sql.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	terms, e := glossary.List(ctx, tx, w, p)
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	out := &RequirementGlossary{Version: a.Version}
	for _, m := range glossary.Matches(source, terms) {
		v := &GlossaryMatch{Start: int32(m.Start), End: int32(m.End), Text: m.Text}
		for _, t := range m.Terms {
			v.Terms = append(v.Terms, glossaryTerm(t))
		}
		out.Matches = append(out.Matches, v)
	}
	out.Candidates = unconfirmedCandidates(source, a.RequirementId, a.Version, terms)
	return out, nil
}
func unconfirmedCandidates(source string, id graphql.ID, version int32, terms []glossary.Term) []*GlossaryCandidate {
	var result []*GlossaryCandidate
	for _, c := range glossary.Candidates(source) {
		known := false
		for _, t := range terms {
			if strings.EqualFold(t.Name, c.Name) {
				for _, v := range t.Variants {
					if strings.EqualFold(v.Text, c.Abbreviation) {
						known = true
						break
					}
				}
			}
		}
		if !known {
			result = append(result, &GlossaryCandidate{SourceId: id, SourceVersion: version, Name: c.Name, Abbreviation: c.Abbreviation})
		}
	}
	return result
}

func (r *Resolver) GlossaryCandidates(ctx context.Context, a GlossaryCandidatesArgs) (*GlossaryCandidatePage, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if a.Offset < 0 || a.Limit < 1 || a.Limit > 100 {
		return nil, errors.New("invalid pagination")
	}
	w, p := workspace(ctx), string(a.ProjectId)
	terms, e := glossary.List(ctx, r.DB, w, p)
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT id,version,text FROM requirements WHERE workspace=$1 AND project_id=$2 ORDER BY created_at,id LIMIT $3 OFFSET $4`, w, p, a.Limit+1, a.Offset)
	if e != nil {
		return nil, errors.New("glossary unavailable")
	}
	defer rows.Close()
	out := &GlossaryCandidatePage{}
	for n := int32(0); rows.Next(); n++ {
		if n == a.Limit {
			out.HasMore = true
			break
		}
		var id graphql.ID
		var version int32
		var text string
		if e = rows.Scan(&id, &version, &text); e != nil {
			return nil, e
		}
		out.Items = append(out.Items, unconfirmedCandidates(text, id, version, terms)...)
	}
	return out, rows.Err()
}
