package graph

import (
	"github.com/VolkovDani/project-creator/internal/glossary"
	graphql "github.com/graph-gophers/graphql-go"
)

func pairGlossaryHints(hints []glossary.Hint) []*PairGlossaryHint {
	out := make([]*PairGlossaryHint, 0, len(hints))
	for _, hint := range hints {
		view := &PairGlossaryHint{Spelling: hint.Spelling, Ambiguous: hint.Ambiguous}
		if !hint.Ambiguous {
			id, version := graphql.ID(hint.TermID), int32(hint.Version)
			view.TermId, view.Version = &id, &version
			view.Name = &hint.Name
			view.Definition = &hint.Definition
			view.Context = &hint.Context
		}
		out = append(out, view)
	}
	return out
}
