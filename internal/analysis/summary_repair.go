package analysis

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Only constrain the corrective generation. Its result still passes all normal
// checks, including whole identifiers, source numbers and configured-language prose.
func (p SummaryLanguagePolicy) repairPattern(source string) string {
	names, labels := p.sourceNames(source)
	literals := map[string]bool{}
	for _, word := range summaryWord.FindAllString(source, -1) {
		if names[strings.ToLower(word)] {
			literals[word] = true
		}
	}
	for label := range labels {
		literals[label] = true
	}
	options := make([]string, 0, len(literals)+1)
	for literal := range literals {
		options = append(options, regexp.QuoteMeta(literal))
	}
	sort.Strings(options)
	var class strings.Builder
	const common = " 0123456789.,:;!?/%()+=_«»“”…—–-"
	class.WriteString("[ 0-9.,:;!?/%()+=_«»“”…—–")
	// Put source units and punctuation in the same character class. Duplicate
	// alternatives inside a repetition create ambiguity for the grammar sampler.
	var symbols []rune
	seen := map[rune]bool{}
	for _, r := range source {
		if !seen[r] && !strings.ContainsRune(common, r) && (unicode.IsSymbol(r) || unicode.IsPunct(r) || unicode.IsNumber(r)) {
			seen[r] = true
			symbols = append(symbols, r)
		}
	}
	sort.Slice(symbols, func(i, j int) bool { return symbols[i] < symbols[j] })
	for _, r := range symbols {
		if strings.ContainsRune(`[]^\`, r) {
			class.WriteByte('\\')
		}
		class.WriteRune(r)
	}
	// Compress the same alphabet tables used for validation into regex ranges.
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
	class.WriteString("-]")
	options = append(options, class.String())
	return "^(" + strings.Join(options, "|") + ")+$"
}
