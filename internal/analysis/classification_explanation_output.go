package analysis

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/explanation"
)

const (
	explanationNoteMax               = 400
	explanationQuoteMax              = 600
	explanationReasonMax             = 800
	explanationWordingMax            = 1200
	explanationQuestionMax           = 300
	explanationQuestionsMax          = 4
	explanationNarrativeLimit        = 180
	managedExplanationNarrativeLimit = 64
)

var explanationVerdicts = []string{"fits", "partial", "insufficient", "does_not_fit"}
var explanationStatuses = []string{"present", "missing", "uncertain", "not_applicable"}

const explanationProseCharacters = `[^"\\\r\n]`

// Printable ASCII except quote and backslash, Latin and Cyrillic letters with
// combining accents, and common typographic punctuation and currency symbols.
const explanationRestrictedProseCharacters = "[ !#-\\[\\]-~" +
	"\u00a0-\u036f\u0400-\u052f\u1e00-\u1eff" +
	"\u2010-\u203f\u20a0-\u20bf№−]"

func classificationExplanationSchema(meaning, form []string, locale string, lastPassage int) map[string]any {
	return explanationSchemaForProvider(meaning, form, locale, lastPassage, false, explanationProseCharacters)
}

func explanationSchemaForProvider(meaning, form []string, locale string, lastPassage int, managed bool, proseCharacters string) map[string]any {
	narrativeLimit := explanationNarrativeLimit
	if managed {
		narrativeLimit = managedExplanationNarrativeLimit
	}
	object := func(props map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": slices.Sorted(maps.Keys(props)), "additionalProperties": false}
	}
	text := func(minLen, maxLen int) map[string]any {
		return map[string]any{"type": "string", "minLength": minLen, "maxLength": maxLen}
	}
	narrative := func(minLen, maxLen int) map[string]any {
		// The local grammar engine consumes the pattern when one is present.
		// Bound it directly and keep JSON delimiters outside the narrative.
		maxLen = min(maxLen, narrativeLimit)
		field := text(minLen, maxLen)
		alphabet := "[A-Za-z]"
		if locale == "ru" {
			alphabet = "[А-Яа-яЁё]"
		}
		pattern := alphabet + fmt.Sprintf("%s{0,%d}", proseCharacters, maxLen-1)
		if minLen == 0 {
			pattern = "(" + pattern + ")?"
		}
		field["pattern"] = "^" + pattern + "$"
		return field
	}
	criterion := func(statuses []string) map[string]any {
		status := map[string]any{"type": "string", "enum": statuses}
		note := narrative(1, explanationNoteMax)
		if managed {
			// OpenVINO emits properties in schema order. Decide if the
			// criterion is present before choosing a passage number.
			properties := struct {
				Status   map[string]any `json:"status"`
				Evidence map[string]any `json:"evidence"`
				Note     map[string]any `json:"note"`
			}{status, map[string]any{"type": "integer"}, note}
			return map[string]any{
				"type": "object", "properties": properties, "required": []string{"status", "evidence", "note"}, "additionalProperties": false,
			}
		}
		return object(map[string]any{
			"status":   status,
			"evidence": map[string]any{"type": "integer", "minimum": 0, "maximum": lastPassage},
			"note":     note,
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
	questionsMax := explanationQuestionsMax
	if managed {
		questionsMax = 2
	}
	return object(map[string]any{"content": axis(meaning), "form": axis(form), "limitations": narrative(1, explanationReasonMax),
		"wording": narrative(0, explanationWordingMax), "questions": map[string]any{"type": "array", "maxItems": questionsMax, "items": narrative(1, explanationQuestionMax)}})
}

// Each nonzero index identifies one contiguous source passage. Keep separators
// so the model can read the whole source in order. Trim only the
// outside whitespace when displaying a selected passage.
func explanationPassages(source string) []string {
	passages := []string{""}
	text := []rune(source)
	for len(text) > 0 {
		start := 0
		for start < len(text) && unicode.IsSpace(text[start]) {
			start++
		}
		end := min(start+explanationQuoteMax, len(text))
		lastSpace := -1
		for i := start; i < end; i++ {
			r := text[i]
			if r == '\n' || (strings.ContainsRune(".!?;。！？", r) && i+1 < len(text) && unicode.IsSpace(text[i+1])) {
				end = i + 1
				break
			}
			if unicode.IsSpace(r) {
				lastSpace = i
			}
			if i+1 == end && end < len(text) && !unicode.IsSpace(text[end]) && lastSpace > start {
				end = lastSpace
			}
		}
		for end < len(text) && unicode.IsSpace(text[end]) {
			end++
		}
		passages = append(passages, string(text[:end]))
		text = text[end:]
	}
	return passages
}

const errClassificationOutput invalidModelOutput = "classification explanation does not match the selected criteria or source"

type explanationOutputCriterion struct {
	Status   string `json:"status"`
	Evidence *int   `json:"evidence"`
	Note     string `json:"note"`
}
type explanationOutputAxis struct {
	Verdict  string                                `json:"verdict"`
	Reason   string                                `json:"reason"`
	Criteria map[string]explanationOutputCriterion `json:"criteria"`
}

func (axis explanationOutputAxis) assessment(passages []string, category, locale string, managed bool) (explanation.Assessment, error) {
	keys := explanationCriterionKeys(category, managed)
	if len(keys) == 0 || len(axis.Criteria) != len(keys) || !slices.Contains(explanationVerdicts, axis.Verdict) || !explanationNarrative(axis.Reason, locale, 1, explanationReasonMax) {
		return explanation.Assessment{}, errClassificationOutput
	}
	result := explanation.Assessment{Reason: axis.Reason, Verdict: axis.Verdict}
	for _, key := range keys {
		c, ok := axis.Criteria[key]
		if !ok || !slices.Contains(explanationStatuses, c.Status) || !explanationNarrative(c.Note, locale, 1, explanationNoteMax) || c.Evidence == nil || *c.Evidence < 0 || *c.Evidence >= len(passages) {
			return explanation.Assessment{}, errClassificationOutput
		}
		quote := strings.TrimSpace(passages[*c.Evidence])
		if quote == "" && c.Status == "present" {
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

func decodeClassificationExplanation(content, source string, target explanation.Target, passages []string) (explanation.Result, error) {
	return decodeExplanationForProvider(content, source, target, passages, false)
}

func decodeExplanationForProvider(content, source string, target explanation.Target, passages []string, managed bool) (explanation.Result, error) {
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
	meaning, err := raw.Meaning.assessment(passages, target.Semantic, target.Locale, managed)
	if err != nil {
		return empty, err
	}
	form, err := raw.Form.assessment(passages, target.Representation, target.Locale, managed)
	if err != nil {
		return empty, err
	}
	if meaning.Verdict == "fits" && form.Verdict == "fits" {
		return explanation.Result{Meaning: meaning, Form: form, Questions: []string{}, Limitations: explanationFitTexts()[target.Locale].Limit}, nil
	}
	return explanation.Result{Meaning: meaning, Form: form, Questions: raw.Questions, Wording: groundedExplanationWording(*raw.Wording, source), Limitations: raw.Limitations}, nil
}

// Verify each narrative field independently. Latin technical identifiers may
// occur in Russian prose. Evidence is checked separately against the original.
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
	if explanationHasUnsupportedLetter(text) {
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

func explanationHasUnsupportedLetter(text string) bool {
	return strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsLetter(r) && !unicode.In(r, unicode.Latin, unicode.Cyrillic)
	})
}

func explanationResponseHasUnsupportedLetter(content string) bool {
	decoder := json.NewDecoder(strings.NewReader(content))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if field, ok := token.(string); ok && explanationHasUnsupportedLetter(field) {
			return true
		}
	}
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
