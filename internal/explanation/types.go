// Package explanation assesses the fit of a saved classification against user-selected targets.
package explanation

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/VolkovDani/project-creator/contracts"
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

type Axis string

const (
	MeaningAxis Axis = "meaning"
	FormAxis    Axis = "form"
)

type AxisResult struct {
	Assessment  Assessment `json:"assessment"`
	Questions   []string   `json:"questions"`
	Wording     string     `json:"wording"`
	Limitations string     `json:"limitations"`
}

func CombineAxes(meaning, form AxisResult, locale string) Result {
	result := Result{Meaning: meaning.Assessment, Form: form.Assessment, Questions: []string{}}
	var limitations []string
	for _, axis := range [2]AxisResult{meaning, form} {
		if axis.Assessment.Verdict == "fits" {
			continue
		}
		for _, question := range axis.Questions {
			if !slices.Contains(result.Questions, question) {
				result.Questions = append(result.Questions, question)
			}
		}
		if !slices.Contains(limitations, axis.Limitations) {
			limitations = append(limitations, axis.Limitations)
		}
		// Prefer a grounded wording suggestion from the assessment of form.
		if axis.Wording != "" {
			result.Wording = axis.Wording
		}
	}
	result.Limitations = strings.Join(limitations, " ")
	if result.Limitations == "" {
		result.Limitations = contracts.Dictionary(locale)["explanation.sourceOnlyLimit"]
	}
	return result
}

// Generators return these errors when a request exceeds the model context or
// when the generated assessment fails validation.
var (
	ErrInputTooLong  = errors.New("explanation input exceeds model context")
	ErrInvalidOutput = errors.New("invalid explanation output")
)

type Generator interface {
	ExplanationProfile() string
	// ExplainClassificationAxes returns validated results for the requested axes.
	// Results completed before an error may be included.
	ExplainClassificationAxes(context.Context, Source, Target, []Axis) (map[Axis]AxisResult, error)
}
