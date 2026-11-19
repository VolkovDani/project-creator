package graph

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/VolkovDani/project-creator/internal/glossary"
)

func (r *Resolver) SuggestGlossaryDefinition(ctx context.Context, a SuggestGlossaryDefinitionArgs) (*GlossaryDefinitionSuggestion, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	in := a.Input
	name, errs := glossary.DraftName(in.Name)
	if len(errs) > 0 {
		return &GlossaryDefinitionSuggestion{Status: "invalid", FieldErrors: fieldErrors(errs)}, nil
	}

	const scope = "FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4"
	args := []any{workspace(ctx), in.ProjectId, in.SourceId, in.SourceVersion}
	var source string
	err := r.DB.QueryRowContext(ctx, "SELECT text "+scope, args...).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return &GlossaryDefinitionSuggestion{Status: "stale"}, nil
	}
	if err != nil {
		return nil, errors.New("glossary unavailable")
	}

	candidates := glossary.Candidates(source)
	index := slices.IndexFunc(candidates, func(candidate glossary.Candidate) bool {
		return candidate.Start == int(in.Start) && candidate.End == int(in.End)
	})
	if index < 0 {
		return &GlossaryDefinitionSuggestion{Status: "stale"}, nil
	}
	candidate := candidates[index]
	if r.Definitions == nil {
		return &GlossaryDefinitionSuggestion{Status: "unavailable"}, nil
	}
	if name == "" {
		name = candidate.Name
	}
	draft, err := r.Definitions.SuggestDefinition(ctx, glossary.DefinitionRequest{
		Text: source, Name: name, Abbreviation: candidate.Abbreviation, Start: candidate.Start, End: candidate.End,
	})
	if err != nil {
		return &GlossaryDefinitionSuggestion{Status: "unavailable"}, nil
	}
	// Inference can outlast an edit or deletion. Only return a draft for the current source.
	var current bool
	if err := r.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT "+scope+")", args...).Scan(&current); err != nil {
		return nil, errors.New("glossary unavailable")
	}
	if !current {
		return &GlossaryDefinitionSuggestion{Status: "stale"}, nil
	}
	if draft.Text == "" {
		return &GlossaryDefinitionSuggestion{Status: "insufficient"}, nil
	}
	status := "suggested"
	if draft.Hypothesis {
		status = "hypothesis"
	}
	return &GlossaryDefinitionSuggestion{Status: status, Definition: &draft.Text}, nil
}
