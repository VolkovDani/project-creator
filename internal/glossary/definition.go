package glossary

import "context"

// DefinitionRequest describes a candidate in the authorized requirement version.
// Start and End are Unicode code point offsets, with an exclusive End.
type DefinitionRequest struct {
	Text, Abbreviation, Name string
	Start, End               int
}

// DefinitionGenerator proposes draft text. An empty result without an error
// means that the source does not supply enough evidence for a definition.
type DefinitionGenerator interface {
	SuggestDefinition(context.Context, DefinitionRequest) (string, error)
}
