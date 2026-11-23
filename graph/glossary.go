package graph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VolkovDani/project-creator/internal/glossary"
	graphql "github.com/graph-gophers/graphql-go"
	"slices"
	"strings"
)

func glossaryTerm(t glossary.Term) *GlossaryTerm {
	v := &GlossaryTerm{ID: graphql.ID(t.ID), Name: t.Name, Definition: t.Definition, Context: t.Context, Version: int32(t.Version), SourceId: graphql.ID(t.SourceID), SourceVersion: int32(t.SourceVersion), UpdatedAt: t.UpdatedAt, CreatedAt: t.CreatedAt}
	for _, a := range t.Variants {
		v.Variants = append(v.Variants, &GlossaryVariant{Text: a.Text, Kind: a.Kind})
	}
	return v
}
func (r *Resolver) GlossaryTermsByIds(ctx context.Context, args GlossaryTermsByIdsArgs) ([]*GlossaryTerm, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if len(args.Ids) < 1 || len(args.Ids) > 100 {
		return nil, errors.New("invalid glossary term IDs")
	}
	ids := make([]string, len(args.Ids))
	for i, id := range args.Ids {
		ids[i] = string(id)
		if strings.TrimSpace(ids[i]) == "" {
			return nil, errors.New("invalid glossary term IDs")
		}
	}
	terms, err := glossary.TermsByIDs(ctx, r.DB, workspace(ctx), string(args.ProjectId), ids)
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	out := make([]*GlossaryTerm, 0, len(terms))
	for _, term := range terms {
		out = append(out, glossaryTerm(term))
	}
	return out, nil
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
	if a.AnalysisId == nil {
		e = tx.QueryRowContext(ctx, `SELECT text FROM active_requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4`, w, p, a.RequirementId, a.Version).Scan(&source)
	} else {
		e = tx.QueryRowContext(ctx, `SELECT s.source->>'text' FROM analysis_sources s JOIN analysis_runs r ON r.id=s.run_id
WHERE r.workspace=$1 AND r.project_id=$2 AND r.id=$3 AND s.requirement_id=$4 AND (s.source->>'version')::int=$5`, w, p, *a.AnalysisId, a.RequirementId, a.Version).Scan(&source)
	}
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
	out.Matches = glossaryMatches(glossary.Matches(source, terms))
	out.Candidates = unconfirmedCandidates(source, a.RequirementId, a.Version, terms)
	return out, nil
}
func glossaryMatches(matches []glossary.Match) []*GlossaryMatch {
	var out []*GlossaryMatch
	for _, m := range matches {
		v := &GlossaryMatch{Start: int32(m.Start), End: int32(m.End), Text: m.Text}
		for _, t := range m.Terms {
			v.Terms = append(v.Terms, glossaryTerm(t))
		}
		out = append(out, v)
	}
	return out
}

func knownGlossaryCandidate(c glossary.Candidate, terms []glossary.Term) bool {
	for _, t := range terms {
		if c.Name == "" && strings.EqualFold(t.Name, c.Abbreviation) {
			return true
		}
		if c.Name == "" || strings.EqualFold(t.Name, c.Name) {
			for _, v := range t.Variants {
				if strings.EqualFold(v.Text, c.Abbreviation) {
					return true
				}
			}
		}
	}
	return false
}

func candidateResult(c glossary.Candidate, id graphql.ID, version int32) *GlossaryCandidate {
	return &GlossaryCandidate{
		Name: c.Name, Abbreviation: c.Abbreviation, SourceId: id, SourceVersion: version,
		Before: c.Before, After: c.After, Start: int32(c.Start), End: int32(c.End),
		BeforeTruncated: c.BeforeTruncated, AfterTruncated: c.AfterTruncated,
	}
}

func unconfirmedCandidates(source string, id graphql.ID, version int32, terms []glossary.Term) []*GlossaryCandidate {
	var result []*GlossaryCandidate
	for _, c := range glossary.Candidates(source) {
		if !knownGlossaryCandidate(c, terms) {
			result = append(result, candidateResult(c, id, version))
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
	rows, e := r.DB.QueryContext(ctx, `SELECT id,version,text FROM active_requirements WHERE workspace=$1 AND project_id=$2 ORDER BY created_at,id LIMIT $3 OFFSET $4`, w, p, a.Limit+1, a.Offset)
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

func glossaryPageArgs(kind, scope string, first *int32, after *string) (int, connectionCursor, error) {
	size, err := connectionSize(first)
	var cursor connectionCursor
	if err == nil && after != nil {
		cursor, err = decodeConnectionCursor(*after, kind, scope)
	}
	return size, cursor, err
}

func (r *Resolver) GlossaryTermsConnection(ctx context.Context, a GlossaryTermsConnectionArgs) (*GlossaryTermsPage, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	w, p := workspace(ctx), string(a.ProjectId)
	scope := w + "\x00" + p + "\x00" + a.Search
	size, c, err := glossaryPageArgs("terms", scope, a.First, a.After)
	if err != nil {
		return nil, err
	}
	terms, err := glossary.TermPage(ctx, r.DB, w, p, a.Search, c.Key, c.ID, size+1)
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	out := &GlossaryTermsPage{PageInfo: &PageInfo{HasNextPage: len(terms) > size}}
	for _, term := range terms[:min(len(terms), size)] {
		cursor := encodeConnectionCursor("terms", scope, term.Name, term.ID)
		out.Items = append(out.Items, glossaryTerm(term))
		out.PageInfo.EndCursor = &cursor
	}
	return out, nil
}

func (r *Resolver) GlossaryCandidatesConnection(ctx context.Context, a GlossaryCandidatesConnectionArgs) (*GlossaryMatchesPage, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	w, p := workspace(ctx), string(a.ProjectId)
	scope := w + "\x00" + p
	size, c, err := glossaryPageArgs("candidates", scope, a.First, a.After)
	if err != nil {
		return nil, err
	}
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	defer tx.Rollback()
	terms, err := glossary.List(ctx, tx, w, p)
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	out := &GlossaryMatchesPage{PageInfo: &PageInfo{}}
	// Stream scoped source rows; retain only the requested candidates and one lookahead.
	rows, err := tx.QueryContext(ctx, `SELECT id,version,text FROM active_requirements WHERE workspace=$1 AND project_id=$2 AND id >= $3 ORDER BY id`, w, p, c.ID)
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var id graphql.ID
		var version int32
		var text string
		if err := rows.Scan(&id, &version, &text); err != nil {
			return nil, err
		}
		// Use the position in the unfiltered extraction so confirming an earlier match
		// does not change the cursor for subsequent matches in this source.
		for index, match := range glossary.Candidates(text) {
			position := fmt.Sprintf("%010d", index)
			if string(id) == c.ID && position <= c.Key || knownGlossaryCandidate(match, terms) {
				continue
			}
			if len(out.Items) == size {
				out.PageInfo.HasNextPage = true
				return out, nil
			}
			cursor := encodeConnectionCursor("candidates", scope, position, string(id))
			out.Items = append(out.Items, candidateResult(match, id, version))
			out.PageInfo.EndCursor = &cursor
		}
	}
	return out, rows.Err()
}

func (r *Resolver) GlossaryMentions(ctx context.Context, a GlossaryMentionsArgs) (*GlossaryMentionPage, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	w, p, id := workspace(ctx), string(a.ProjectId), string(a.TermId)
	scope := w + "\x00" + p + "\x00" + id
	size, c, err := glossaryPageArgs("mentions", scope, a.First, a.After)
	if err != nil {
		return nil, err
	}
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	defer tx.Rollback()
	terms, err := glossary.List(ctx, tx, w, p)
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	out := &GlossaryMentionPage{PageInfo: &PageInfo{}}
	if !slices.ContainsFunc(terms, func(term glossary.Term) bool { return term.ID == id }) {
		return out, nil
	}
	matches := glossary.MentionsTerm(terms, id)
	rows, err := tx.QueryContext(ctx, `SELECT id,project_id,text,version,created_at,updated_at FROM active_requirements WHERE workspace=$1 AND project_id=$2 AND id>$3 ORDER BY id`, w, p, c.ID)
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		row := &Requirement{}
		if err := rows.Scan(&row.ID, &row.ProjectId, &row.Text, &row.Version, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		if !matches(row.Text) {
			continue
		}
		if len(out.Items) == size {
			out.PageInfo.HasNextPage = true
			break
		}
		out.Items = append(out.Items, row)
		cursor := encodeConnectionCursor("mentions", scope, id, string(row.ID))
		out.PageInfo.EndCursor = &cursor
	}
	return out, rows.Err()
}
