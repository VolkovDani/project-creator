package processgraph

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

func relationshipEvidence(from, to Node, edge SuggestedEdge, evidence []Evidence) []Evidence {
	left, right := labelWords(from.Label), labelWords(to.Label)
	if placeholderLabel(left) || placeholderLabel(right) || slices.Equal(left, right) {
		return nil
	}
	var supported []Evidence
	for _, span := range adjacentCitations(evidence) {
		var fragments []string
		for _, item := range span {
			fragments = append(fragments, item.Quote)
		}
		text := strings.Join(fragments, " ")
		words := labelWords(text)
		start, other := sourceLabelPosition(left, words), sourceLabelPosition(right, words)
		if start < 0 || other < 0 || !supportsLabel(edge.Label, text) {
			continue
		}
		if edge.Kind != "follows" || explicitOrder(words, start, other) {
			supported = append(supported, span...)
		}
	}
	return supported
}

// A single edge must be supported within one cited source passage. Join nearby
// fragments of that passage so a term split by punctuation remains visible.
func adjacentCitations(evidence []Evidence) [][]Evidence {
	ordered := slices.Clone(evidence)
	slices.SortFunc(ordered, func(a, b Evidence) int {
		return cmp.Or(cmp.Compare(a.RequirementID, b.RequirementID), cmp.Compare(a.Version, b.Version), cmp.Compare(a.Start, b.Start))
	})
	var spans [][]Evidence
	var previous Evidence
	for i, item := range ordered {
		if i > 0 && item.RequirementID == previous.RequirementID && item.Version == previous.Version &&
			item.Start <= previous.End+2 {
			spans[len(spans)-1] = append(spans[len(spans)-1], item)
		} else {
			spans = append(spans, []Evidence{item})
		}
		if previous.RequirementID != item.RequirementID || previous.Version != item.Version || item.End > previous.End {
			previous = item
		}
	}
	return spans
}

func labelWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsNumber(r) && r != '_'
	})
}

func placeholderLabel(words []string) bool {
	switch strings.Join(words, " ") {
	case "", "следующий шаг", "следующее действие", "предыдущий шаг", "предыдущее действие",
		"действие", "субъект", "роль", "результат", "условие", "состояние", "событие",
		"я", "он", "она", "они", "мы", "i", "he", "she", "it", "we", "they",
		"next step", "next action", "previous step", "previous action", "action", "actor", "role",
		"result", "condition", "state", "event":
		return true
	}
	return false
}

func sourceLabelPosition(label, source []string) int {
	if len(label) == 0 {
		return -1
	}
	for i := 0; i+len(label) <= len(source); i++ {
		part := source[i:]
		if matchesSourcePhrase(label, part) {
			return i
		}
		if len(label) == 2 && matchesSourcePhrase([]string{label[1], label[0]}, part) {
			return i
		}
	}
	return -1
}

func matchesSourcePhrase(label, source []string) bool {
	position := 0
	for i, word := range label {
		if position < len(source) && !groundedWord(word, source[position]) && i > 0 {
			switch source[position] {
			case "a", "an", "the", "вправе":
				position++
			}
		}
		if position >= len(source) || !groundedWord(word, source[position]) {
			return false
		}
		position++
	}
	return true
}

func explicitOrder(words []string, from, to int) bool {
	for i, word := range words {
		switch word {
		case "затем", "потом", "далее", "then":
			if from < i && i < to {
				return true
			}
		case "после", "after":
			if word == "после" && i+1 < len(words) && words[i+1] == "чего" && from < i && i+1 < to {
				return true
			}
			if i+1 == from && from < to || to < i && i+1 == from {
				return true
			}
		case "перед", "before":
			if from < i && i+1 == to || i+1 == to && to < from {
				return true
			}
		}
	}
	return false
}

var russianEndingGroups = [][]string{
	{"", "а", "я", "ы", "и", "у", "ю", "е", "ом", "ем", "ой", "ей", "ам", "ям", "ах", "ях", "ов", "ев", "ами", "ями"},
	{"ый", "ий", "ой", "ая", "яя", "ое", "ее", "ые", "ие", "ого", "его", "ому", "ему", "ым", "им", "ую", "юю", "ых", "их", "ыми", "ими"},
	{"ть", "ет", "ют", "ем", "ете", "л", "ла", "ли", "ло", "й", "йте"},
	{"ить", "ит", "ил", "ила", "или", "ило", "им", "ите", "ат", "ят"},
}

var englishEndings = []string{"", "s", "es", "d", "ed", "ing"}

func groundedWord(label, source string) bool {
	if label == source {
		return true
	}
	if label == "требует" && source == "требуется" || label == "требуется" && source == "требует" {
		return true
	}
	if simpleLetters(label, unicode.Cyrillic) && simpleLetters(source, unicode.Cyrillic) {
		return slices.ContainsFunc(russianEndingGroups, func(endings []string) bool {
			return shareWordBase(label, source, endings, 3)
		})
	}
	if simpleLetters(label, unicode.Latin) && simpleLetters(source, unicode.Latin) {
		return shareWordBase(label, source, englishEndings, 4)
	}
	return false
}

func simpleLetters(word string, script *unicode.RangeTable) bool {
	return word != "" && strings.IndexFunc(word, func(r rune) bool { return !unicode.Is(script, r) }) < 0
}

// Match regular endings conservatively. This check keeps lexical evidence and
// does not perform synonym replacement or merge confirmed concepts.
func shareWordBase(a, b string, endings []string, minimum int) bool {
	for _, ending := range endings {
		base, ok := strings.CutSuffix(a, ending)
		if !ok || len([]rune(base)) < minimum {
			continue
		}
		tail, ok := strings.CutPrefix(b, base)
		if ok && slices.Contains(endings, tail) {
			return true
		}
	}
	return false
}
