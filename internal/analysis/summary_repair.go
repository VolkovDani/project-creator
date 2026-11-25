package analysis

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A word uses either the native alphabet or one complete source spelling.
// Separate the alternatives by non-word characters so the decoder cannot
// splice an identifier into native prose or invent one from a source prefix.
// Require a native letter outside identifiers so copying them alone cannot
// satisfy the grammar.
func (p SummaryLanguagePolicy) repairPattern(source string, requiredNumbers ...string) string {
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
	if len(requiredNumbers) > 0 {
		// Start with native wording and allow prose around each quoted source value.
		var pattern strings.Builder
		pattern.WriteString("^" + sep + "*" + nativeWord)
		moreWords := "(" + sep + "+" + word + ")*"
		for _, number := range requiredNumbers {
			pattern.WriteString(moreWords + sep + "+" + regexp.QuoteMeta(number) + `\.?`)
			if pattern.Len() > summaryContextBytes {
				return ""
			}
		}
		pattern.WriteString(moreWords + sep + "*$")
		return pattern.String()
	}
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

// summaryRepairNumbers keeps complete source tokens when a number belongs to
// an identifier, a date, or a unit. Decimal commas may span two word tokens.
func summaryRepairNumbers(source string) []string {
	words := summaryWord.FindAllStringIndex(source, -1)
	var spans [][2]int
	word := 0
	for _, number := range summaryNumbers.FindAllStringIndex(source, -1) {
		for word < len(words) && words[word][1] <= number[0] {
			word++
		}
		start, end := number[0], number[1]
		for i := word; i < len(words) && words[i][0] < end; i++ {
			start, end = min(start, words[i][0]), max(end, words[i][1])
		}
		if source[start] >= '0' && source[start] <= '9' {
			// The publication number scan does not include a unary sign or a
			// leading decimal point. Keep their exact spelling in the grammar.
			if start > 0 && (source[start-1] == '.' || source[start-1] == ',') {
				start--
			}
			if start > 0 {
				sign, size := utf8.DecodeLastRuneInString(source[:start])
				if strings.ContainsRune("+-−", sign) {
					start -= size
				}
			}
		}
		if count := len(spans); count > 0 && spans[count-1][1] > start {
			spans[count-1][1] = max(spans[count-1][1], end)
		} else {
			spans = append(spans, [2]int{start, end})
		}
	}

	required := make([]string, 0, len(spans))
	seen := make(map[string]bool, len(spans))
	for _, span := range spans {
		value := source[span[0]:span[1]]
		if !seen[value] {
			required = append(required, value)
			seen[value] = true
		}
	}
	return required
}
