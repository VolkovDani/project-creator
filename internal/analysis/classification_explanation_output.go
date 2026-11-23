package analysis

import (
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/explanation"
)

const (
	explanationNoteMax      = 400
	explanationQuoteMax     = 600
	explanationReasonMax    = 800
	explanationWordingMax   = 1200
	explanationQuestionMax  = 300
	explanationQuestionsMax = 4
)

var explanationVerdicts = []string{"fits", "partial", "insufficient", "does_not_fit"}
var explanationStatuses = []string{"present", "missing", "uncertain", "not_applicable"}

func classificationExplanationSchema(meaning, form []string, locale string) map[string]any {
	object := func(props map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": slices.Sorted(maps.Keys(props)), "additionalProperties": false}
	}
	text := func(minLen, maxLen int) map[string]any {
		return map[string]any{"type": "string", "minLength": minLen, "maxLength": maxLen}
	}
	narrative := func(minLen, maxLen int) map[string]any {
		field := text(minLen, maxLen)
		alphabet := "[A-Za-z]"
		if locale == "ru" {
			alphabet = "[А-Яа-яЁё]"
		}
		pattern := alphabet + ".*"
		if minLen == 0 {
			pattern = "(" + pattern + ")?"
		}
		field["pattern"] = "^" + pattern + "$"
		return field
	}
	criterion := func(statuses []string) map[string]any {
		return object(map[string]any{
			"status":   map[string]any{"type": "string", "enum": statuses},
			"evidence": text(0, explanationQuoteMax), "note": narrative(1, explanationNoteMax),
		})
	}
	axis := func(keys []string) map[string]any {
		checklist := make(map[string]any, len(keys))
		for _, key := range keys {
			statuses := explanationStatuses
			if key != "acceptance" {
				statuses = []string{"present", "missing", "uncertain"}
			}
			checklist[key] = criterion(statuses)
		}
		return object(map[string]any{"verdict": map[string]any{"type": "string", "enum": explanationVerdicts},
			"reason": narrative(1, explanationReasonMax), "criteria": object(checklist)})
	}
	return object(map[string]any{"content": axis(meaning), "form": axis(form), "limitations": narrative(1, explanationReasonMax),
		"wording": narrative(0, explanationWordingMax), "questions": map[string]any{"type": "array", "maxItems": explanationQuestionsMax, "items": narrative(1, explanationQuestionMax)}})
}

const errClassificationOutput invalidModelOutput = "classification explanation does not match the selected criteria or source"

type explanationOutputCriterion struct {
	Status   string  `json:"status"`
	Evidence *string `json:"evidence"`
	Note     string  `json:"note"`
}
type explanationOutputAxis struct {
	Verdict  string                                `json:"verdict"`
	Reason   string                                `json:"reason"`
	Criteria map[string]explanationOutputCriterion `json:"criteria"`
}

func (axis explanationOutputAxis) assessment(source, category, locale string) (explanation.Assessment, error) {
	keys := explanationCriteria[category]
	if len(keys) == 0 || len(axis.Criteria) != len(keys) || !slices.Contains(explanationVerdicts, axis.Verdict) || !explanationNarrative(axis.Reason, locale, 1, explanationReasonMax) {
		return explanation.Assessment{}, errClassificationOutput
	}
	result := explanation.Assessment{Reason: axis.Reason, Verdict: axis.Verdict}
	for _, key := range keys {
		c, ok := axis.Criteria[key]
		if !ok || !slices.Contains(explanationStatuses, c.Status) || !explanationNarrative(c.Note, locale, 1, explanationNoteMax) || c.Evidence == nil || !explanationString(*c.Evidence, 0, explanationQuoteMax) {
			return explanation.Assessment{}, errClassificationOutput
		}
		quote := *c.Evidence
		if (quote != "" && !strings.Contains(source, quote)) || (quote == "" && c.Status == "present") {
			return explanation.Assessment{}, errClassificationOutput
		}
		// A model may quote actual context while explaining an absent criterion.
		// Only display excerpts for present or uncertain criteria.
		if c.Status == "missing" || c.Status == "not_applicable" {
			quote = ""
		}
		optional := category == "user_story" && key == "acceptance"
		if !optional && (c.Status == "not_applicable" || axis.Verdict == "fits" && c.Status != "present") {
			return explanation.Assessment{}, errClassificationOutput
		}
		result.Criteria = append(result.Criteria, explanation.Criterion{Name: explanationGuides()[locale][category].Criteria[key], Status: c.Status, Evidence: quote, Note: c.Note})
	}
	if result.Verdict == "fits" {
		result.Reason = explanationFitTexts()[locale].Reason
	}
	return result, nil
}

func decodeClassificationExplanation(content, source string, target explanation.Target) (explanation.Result, error) {
	empty := explanation.Result{}
	var raw struct {
		Meaning     explanationOutputAxis `json:"content"`
		Form        explanationOutputAxis `json:"form"`
		Questions   []string              `json:"questions"`
		Wording     *string               `json:"wording"`
		Limitations string                `json:"limitations"`
	}
	d := json.NewDecoder(strings.NewReader(content))
	d.DisallowUnknownFields()
	if !utf8.ValidString(content) || d.Decode(&raw) != nil || d.Decode(new(any)) != io.EOF || raw.Wording == nil || !explanationNarrative(*raw.Wording, target.Locale, 0, explanationWordingMax) ||
		!explanationNarrative(raw.Limitations, target.Locale, 1, explanationReasonMax) || raw.Questions == nil || len(raw.Questions) > explanationQuestionsMax {
		return empty, errClassificationOutput
	}
	for _, q := range raw.Questions {
		if !explanationNarrative(q, target.Locale, 1, explanationQuestionMax) {
			return empty, errClassificationOutput
		}
	}
	meaning, err := raw.Meaning.assessment(source, target.Semantic, target.Locale)
	if err != nil {
		return empty, err
	}
	form, err := raw.Form.assessment(source, target.Representation, target.Locale)
	if err != nil {
		return empty, err
	}
	if meaning.Verdict == "fits" && form.Verdict == "fits" {
		return explanation.Result{Meaning: meaning, Form: form, Questions: []string{}, Limitations: explanationFitTexts()[target.Locale].Limit}, nil
	}
	return explanation.Result{Meaning: meaning, Form: form, Questions: raw.Questions, Wording: groundedExplanationWording(*raw.Wording, source), Limitations: raw.Limitations}, nil
}

// Verify each narrative field independently. Technical identifiers may occur in
// prose; the evidence field is always checked separately against the original.
func explanationNarrative(text, locale string, minLen, maxLen int) bool {
	if !explanationString(text, minLen, maxLen) {
		return false
	}
	if text == "" {
		return true
	}
	native := summaryLanguageCatalog[locale]
	first, _ := utf8.DecodeRuneInString(text)
	if native == nil || !native(first) {
		return false
	}
	localWords, foreignWords := 0, 0
	for _, word := range summaryWord.FindAllString(text, -1) {
		if !hasSummaryLetter(word) {
			continue
		}
		if len(word) > 1 && strings.ToUpper(word) == word && asciiSummaryName(word) {
			continue
		}
		if strings.ContainsFunc(word, func(r rune) bool { return unicode.IsLetter(r) && !native(r) }) {
			foreignWords++
		} else {
			localWords++
		}
	}
	return localWords > 0 && localWords >= foreignWords
}

// Permit rearranging the user's words in a draft, while dropping drafts that
// introduce terms, numbers or actors which cannot be traced to the source.
func groundedExplanationWording(wording, source string) string {
	if wording == "" {
		return ""
	}
	sourceWords := summaryWord.FindAllString(strings.ToLower(source), -1)
	draftWords := summaryWord.FindAllString(strings.ToLower(wording), -1)
	if slices.Equal(sourceWords, draftWords) {
		return ""
	}
	available := make(map[string]int)
	for _, word := range sourceWords {
		available[word]++
	}
	for _, word := range draftWords {
		if available[word] == 0 {
			return ""
		}
		available[word]--
	}
	return wording
}
