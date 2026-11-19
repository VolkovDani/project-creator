package analysis

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/VolkovDani/project-creator/contracts"
)

// SummaryLanguagePolicy limits model summaries to the configured language alphabets.
// Alphabet checks are intentionally approximate and do not identify a language.
type SummaryLanguagePolicy struct {
	codes []string
}

var summaryLanguageCatalog = map[string]func(rune) bool{
	"en": func(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' },
	"ru": func(r rune) bool { return r >= 'А' && r <= 'Я' || r >= 'а' && r <= 'я' || r == 'Ё' || r == 'ё' },
	"de": func(r rune) bool {
		return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("ÄÖÜäöüßẞ", r)
	},
	"fr": func(r rune) bool {
		return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("ÀÂÆÇÉÈÊËÎÏÔŒÙÛÜŸàâæçéèêëîïôœùûüÿ", r)
	},
	"es": func(r rune) bool {
		return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("ÁÉÍÑÓÚÜáéíñóúü", r)
	},
	"uk": func(r rune) bool {
		return (r >= 'А' && r <= 'Я' || r >= 'а' && r <= 'я') && !strings.ContainsRune("ЫыЭэЁёЪъ", r) || strings.ContainsRune("ҐґЄєІіЇї", r)
	},
	// For zh and ja, script ranges are a permissive approximation, not language detection.
	"zh": func(r rune) bool { return unicode.Is(unicode.Han, r) },
	"ja": func(r rune) bool {
		return unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Han, r) || r == 'ー'
	},
}

func NewSummaryLanguagePolicy(csv string) (SummaryLanguagePolicy, error) {
	if strings.TrimSpace(csv) == "" {
		if csv != "" {
			return SummaryLanguagePolicy{}, fmt.Errorf("empty language code")
		}
		csv = "ru,en"
	}
	codes := strings.Split(csv, ",")
	seen := make(map[string]bool, len(codes))
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if code == "" {
			return SummaryLanguagePolicy{}, fmt.Errorf("empty language code")
		}
		if _, ok := summaryLanguageCatalog[code]; !ok {
			return SummaryLanguagePolicy{}, fmt.Errorf("unsupported summary language %q", code)
		}
		seen[code] = true
	}
	canonical := make([]string, 0, len(seen))
	for code := range seen {
		canonical = append(canonical, code)
	}
	sort.Strings(canonical)
	return SummaryLanguagePolicy{codes: canonical}, nil
}

func (p SummaryLanguagePolicy) Codes() []string { return append([]string(nil), p.codes...) }

func (p SummaryLanguagePolicy) Instruction() string {
	names := map[string]string{"en": "English", "ru": "Russian", "de": "German", "fr": "French", "es": "Spanish", "uk": "Ukrainian", "zh": "Chinese", "ja": "Japanese"}
	labels := make([]string, len(p.codes))
	for i, code := range p.codes {
		labels[i] = names[code] + " (" + code + ")"
	}
	return "Write the summary ONLY in these configured languages: " + strings.Join(labels, ", ") + ". Keep the source language when it is allowed; otherwise translate faithfully into the first configured language."
}

func (p SummaryLanguagePolicy) Valid(text string) bool {
	return contracts.ValidSummary(text) && p.validAlphabet(text)
}

func (p SummaryLanguagePolicy) validAlphabet(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) {
			ok := false
			for _, code := range p.codes {
				if summaryLanguageCatalog[code](r) {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
			continue
		}
		if unicode.IsMark(r) {
			if r == 0x20e3 || r == 0xfe0e || r == 0xfe0f {
				continue
			}
			return false
		}
		if unicode.IsControl(r) {
			if unicode.IsSpace(r) {
				continue
			}
			return false
		}
		if unicode.Is(unicode.Cf, r) {
			if r == 0x200d || r == 0xfe0e || r == 0xfe0f || r >= 0xe0020 && r <= 0xe007f {
				continue
			}
			return false
		}
		if !unicode.IsNumber(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Technical names are single ASCII word tokens from an otherwise permitted-language
// source. Internal dots/dashes are part of a name; sentence punctuation is not.
var summaryWord = regexp.MustCompile(`[\p{L}\p{N}_]+(?:[-.][\p{L}\p{N}_]+)*`)

// Match the complete technical label so its individual foreign words cannot be
// reused as prose. Only labels copied verbatim from the source are exempt.
var summaryTechnicalLabel = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,11} \([A-Z][A-Za-z0-9-]*(?: [A-Z][A-Za-z0-9-]*){1,7}\)`)

func sourceCodeName(source string, start, end int) bool {
	before, after := source[:start], source[end:]
	left, right := strings.TrimRight(before, " \t"), strings.TrimLeft(after, " \t")
	typedParameter := strings.HasPrefix(right, ":") && (strings.HasSuffix(left, "(") || strings.HasSuffix(left, ","))
	return typedParameter || strings.HasSuffix(before, "@") || strings.HasPrefix(after, "[") || strings.HasPrefix(after, "(") ||
		strings.HasPrefix(after, "]") || strings.HasSuffix(before, "[") ||
		strings.HasPrefix(right, "=") ||
		strings.HasSuffix(left, "=")
}

// forSource narrows the configured alphabets to those that can represent the
// source prose, including isolated technical names. Overlapping alphabets may
// yield several candidates. Mixed or unsupported prose keeps the configured set.
func (p SummaryLanguagePolicy) forSource(source string) SummaryLanguagePolicy {
	if len(p.codes) <= 1 || !hasSummaryLetter(source) {
		return p
	}
	var matching []string
	for _, code := range p.codes {
		single := SummaryLanguagePolicy{codes: []string{code}}
		if single.ValidForSource(source, source) {
			matching = append(matching, code)
		}
	}
	if len(matching) == 0 || len(matching) == len(p.codes) {
		return p
	}
	return SummaryLanguagePolicy{codes: matching}
}

// ValidForSource preserves isolated source names without enabling foreign prose.
// This is deliberately a token heuristic, not product-name or language detection.
func (p SummaryLanguagePolicy) ValidForSource(text, source string) bool {
	if p.Valid(text) {
		return true
	}
	if !contracts.ValidSummary(text) || !contracts.ValidSummary(source) {
		return false
	}
	identifiers, labels := p.sourceNames(source)
	if len(labels) > 0 {
		text = summaryTechnicalLabel.ReplaceAllStringFunc(text, func(label string) string {
			if labels[label] {
				return " "
			}
			return label
		})
	}
	return p.validWithSourceNames(text, identifiers)
}

func (p SummaryLanguagePolicy) sourceNames(source string) (map[string]bool, map[string]bool) {
	isolated := map[string]bool{}
	identifiers := map[string]bool{}
	var run []string
	hasNativeWord := false
	finishRun := func() {
		if len(run) == 1 && asciiSummaryName(run[0]) {
			isolated[strings.ToLower(run[0])] = true
		}
		run = run[:0]
	}
	for _, span := range summaryWord.FindAllStringIndex(source, -1) {
		token := source[span[0]:span[1]]
		if !hasSummaryLetter(token) {
			continue
		}
		if p.Valid(token) {
			hasNativeWord = true
			finishRun()
		} else {
			run = append(run, token)
			if asciiSummaryName(token) && (strings.IndexFunc(token, unicode.IsDigit) >= 0 || sourceCodeName(source, span[0], span[1])) {
				identifiers[strings.ToLower(token)] = true
			}
		}
	}
	finishRun()
	if hasNativeWord {
		for name := range isolated {
			identifiers[name] = true
		}
	}
	labels := map[string]bool{}
	for _, label := range summaryTechnicalLabel.FindAllString(source, -1) {
		labels[label] = true
		identifiers[strings.ToLower(strings.Fields(label)[0])] = true
	}
	return identifiers, labels
}

func (p SummaryLanguagePolicy) validWithSourceNames(text string, identifiers map[string]bool) bool {
	var masked strings.Builder
	last := 0
	for _, span := range summaryWord.FindAllStringIndex(text, -1) {
		token := text[span[0]:span[1]]
		if p.Valid(token) {
			continue
		}
		if !asciiSummaryName(token) || !identifiers[strings.ToLower(token)] {
			return false
		}
		masked.WriteString(text[last:span[0]])
		masked.WriteByte(' ')
		last = span[1]
	}
	masked.WriteString(text[last:])
	remaining := masked.String()
	// Require actual configured-language wording, not an identifier-only answer.
	return hasSummaryLetter(remaining) && p.Valid(remaining)
}

func hasSummaryLetter(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func asciiSummaryName(text string) bool {
	if !hasSummaryLetter(text) {
		return false
	}
	for _, r := range text {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}
