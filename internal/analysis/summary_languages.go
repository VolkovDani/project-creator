package analysis

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/VolkovDani/project-creator/contracts"
)

// SummaryLanguagePolicy limits model summaries to the configured language alphabets.
// Alphabet checks are intentionally approximate and do not identify a language.
type SummaryLanguagePolicy struct {
	codes   []string
	allowed map[string]bool
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
	allowed := make(map[string]bool, len(seen))
	for code := range seen {
		canonical = append(canonical, code)
		allowed[code] = true
	}
	sort.Strings(canonical)
	return SummaryLanguagePolicy{codes: canonical, allowed: allowed}, nil
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
	if !contracts.ValidSummary(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) {
			ok := false
			for code := range p.allowed {
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
