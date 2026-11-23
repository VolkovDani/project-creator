// Package explanation assesses the fit of a saved classification against user-selected targets.
package explanation

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/classification"
)

type Target struct {
	Semantic       string `json:"semantic"`
	Representation string `json:"representation"`
	Locale         string `json:"locale"`
}
type Request struct {
	ProjectID, RequirementID                  string
	TextVersion, Revision, SuggestionRevision int32
	Target                                    Target
}
type Source struct {
	Text       string                `json:"text"`
	Decision   *classification.Entry `json:"decision,omitempty"`
	Suggestion *classification.Entry `json:"suggestion,omitempty"`
}
type Criterion struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Note     string `json:"note"`
}
type Assessment struct {
	Verdict  string      `json:"verdict"`
	Reason   string      `json:"reason"`
	Criteria []Criterion `json:"criteria"`
}
type Result struct {
	Meaning     Assessment `json:"meaning"`
	Form        Assessment `json:"form"`
	Questions   []string   `json:"questions"`
	Wording     string     `json:"wording"`
	Limitations string     `json:"limitations"`
}

// Generators return these errors when a request exceeds the model context or
// when the generated assessment fails validation.
var (
	ErrInputTooLong  = errors.New("explanation input exceeds model context")
	ErrInvalidOutput = errors.New("invalid explanation output")
)

type Generator interface {
	ExplanationProfile() string
	ExplainClassification(context.Context, Source, Target) (Result, error)
}
