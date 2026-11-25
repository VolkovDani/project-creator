package analysis

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// A word uses either the native alphabet or one complete source spelling.
// Separate the alternatives by non-word characters so the decoder cannot
// splice an identifier into native prose or invent one from a source prefix.
// Require a native letter outside identifiers so copying them alone cannot
// satisfy the grammar.
func (p SummaryLanguagePolicy) repairPattern(source string) string {
	names, labels := p.sourceNames(source)
	literals := map[string]bool{}
	for _, word := range summaryWord.FindAllString(source, -1) {
		if names[strings.ToLower(word)] && !p.Valid(word) {
			literals[word] = true
		}
	}
	for label := range labels {
		for _, word := range summaryWord.FindAllString(label, -1) {
			if !p.Valid(word) {
				literals[word] = true
			}
		}
	}

	// Periods and dashes can belong to a source name. They may also surround
	// one as punctuation, but never join it to another word.
	var native, separator strings.Builder
	native.WriteString("[0-9_.")
	alphabet := p.repairAlphabet()
	native.WriteString(alphabet)
	separator.WriteString("[ ,:;!?/%()+=«»“”…—–")
	seen := map[rune]bool{}
	for _, r := range source {
		if seen[r] || strings.ContainsRune("0123456789_.- ,:;!?/%()+=«»“”…—–", r) {
			continue
		}
		seen[r] = true
		switch {
		case unicode.IsNumber(r):
			native.WriteRune(r)
		case unicode.IsSymbol(r) || unicode.IsPunct(r):
			if strings.ContainsRune(`[]^\`, r) {
				separator.WriteByte('\\')
			}
			separator.WriteRune(r)
		}
	}
	native.WriteString("-]")
	separator.WriteByte(']')
	nativeClass := native.String()
	word := nativeClass + "+"
	if len(literals) > 0 {
		options := make([]string, 0, len(literals))
		for literal := range literals {
			options = append(options, regexp.QuoteMeta(literal))
		}
		sort.Strings(options)
		word = "(" + word + "|[.-]*(" + strings.Join(options, "|") + ")[.-]*)"
	}
	sep := separator.String()
	nativeWord := nativeClass + "*[" + alphabet + "]" + nativeClass + "*"
	return "^" + sep + "*(" + word + sep + "+)*" + nativeWord + "(" + sep + "+" + word + ")*" + sep + "*$"
}

// Compress the same alphabet tables used for publication into regex ranges.
func (p SummaryLanguagePolicy) repairAlphabet() string {
	var class strings.Builder
	var first rune
	for r := rune(1); r <= unicode.MaxRune+1; r++ {
		allowed := false
		for _, code := range p.codes {
			if summaryLanguageCatalog[code](r) {
				allowed = true
				break
			}
		}
		if allowed && first == 0 {
			first = r
		}
		if !allowed && first != 0 {
			class.WriteRune(first)
			if r-1 > first {
				class.WriteRune('-')
				class.WriteRune(r - 1)
			}
			first = 0
		}
	}
	return class.String()
}
