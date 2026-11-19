package glossary

import "context"

// DefinitionRequest describes a candidate in the authorized requirement version.
// Start and End are Unicode code point offsets, with an exclusive End.
type DefinitionRequest struct {
	Text, Abbreviation, Name string
	Start, End               int
}

// DefinitionDraft is a proposal for review. Hypothesis marks an inferred meaning.
type DefinitionDraft struct {
	Text       string
	Hypothesis bool
}

// DefinitionGenerator returns an empty draft when the source provides insufficient evidence.
type DefinitionGenerator interface {
	SuggestDefinition(context.Context, DefinitionRequest) (DefinitionDraft, error)
}
