// Package glossary matches confirmed project vocabulary without rewriting sources.
package glossary

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Variant struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
}
type Term struct {
	ID, Name, Definition, Context, SourceID, CreatedAt, UpdatedAt string
	Version, SourceVersion                                        int
	Variants                                                      []Variant
}
type Match struct {
	Start, End int
	Text       string
	Terms      []Term
}
type Candidate struct {
	Name, Abbreviation, Before, After string
	Start, End                        int // Unicode code point offsets in the source.
}

func word(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '_'
}

// Matches reports rune offsets in the original text. Choose the longest whole
// name at each position and keep every interpretation of an ambiguous name.
func Matches(text string, terms []Term) []Match {
	return compile(terms).matches(text)
}

// MentionsTerm uses the same longest-match and ambiguity rules as source highlighting.
func MentionsTerm(terms []Term, id string) func(string) bool {
	matcher := compile(terms)
	return func(text string) bool {
		for _, m := range matcher.matches(text) {
			for _, t := range m.Terms {
				if t.ID == id {
					return true
				}
			}
		}
		return false
	}
}

type spelling struct {
	runes []rune
	terms []Term
}

type matcher map[rune][]*spelling

// MatchText prepares the glossary once when matching a batch of previews.
func MatchText(terms []Term) func(string) []Match { return compile(terms).matches }

func compile(terms []Term) matcher {
	byFirst := matcher{}
	names := map[string]*spelling{}
	for _, term := range terms {
		labels := []string{term.Name}
		for _, v := range term.Variants {
			labels = append(labels, v.Text)
		}
		seen := map[string]bool{}
		for _, label := range labels {
			key := strings.ToLower(label)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			entry := names[key]
			if entry == nil {
				r := []rune(key)
				entry = &spelling{runes: r}
				names[key] = entry
				byFirst[r[0]] = append(byFirst[r[0]], entry)
			}
			entry.terms = append(entry.terms, term)
		}
	}
	for _, entries := range byFirst {
		sort.Slice(entries, func(i, j int) bool { return len(entries[i].runes) > len(entries[j].runes) })
	}
	return byFirst
}

func (byFirst matcher) matches(text string) []Match {
	original := []rune(text)
	lower := []rune(strings.ToLower(text))
	var result []Match
	for i := 0; i < len(original); {
		end := i
		for _, entry := range byFirst[lower[i]] {
			j := i + len(entry.runes)
			if j > len(lower) || (i > 0 && word(lower[i-1]) && word(lower[i])) || (j < len(lower) && word(lower[j-1]) && word(lower[j])) {
				continue
			}
			if string(lower[i:j]) != string(entry.runes) {
				continue
			}
			result = append(result, Match{i, j, string(original[i:j]), entry.terms})
			end = j
			break
		}
		if end == i {
			end++
		}
		i = end
	}
	return result
}

var expansion = regexp.MustCompile(`([\p{L}]+(?:[ \t]+[\p{L}]+){1,15})[ \t]*\(([\p{Lu}]{2,8})\)`)

var sourceWord = regexp.MustCompile(`[\p{L}\p{M}\p{N}_]+`)

// Candidates prefers explicit expansions to standalone mentions in the same source.
func Candidates(text string) []Candidate {
	names := map[int]string{}
	expanded := map[string]bool{}
	for _, span := range expansion.FindAllStringSubmatchIndex(text, -1) {
		words := strings.Fields(text[span[2]:span[3]])
		abbr := text[span[4]:span[5]]
		length := utf8.RuneCountInString(abbr)
		if len(words) < length {
			continue
		}
		words = words[len(words)-length:]
		var initials strings.Builder
		for _, w := range words {
			r, _ := utf8.DecodeRuneInString(w)
			initials.WriteRune(unicode.ToUpper(r))
		}
		if initials.String() == abbr {
			names[span[4]] = strings.Join(words, " ")
			expanded[abbr] = true
		}
	}
	chars := []rune(text)
	seen := map[[2]string]bool{}
	var result []Candidate
	bytePos, runePos := 0, 0
	for _, span := range sourceWord.FindAllStringIndex(text, -1) {
		token := text[span[0]:span[1]]
		name := names[span[0]]
		if name == "" && expanded[token] {
			continue
		}
		length := utf8.RuneCountInString(token)
		if length < 2 || length > 8 || strings.IndexFunc(token, func(r rune) bool { return !unicode.Is(unicode.Lu, r) }) >= 0 {
			continue
		}
		key := [2]string{strings.ToLower(name), token}
		if seen[key] {
			continue
		}
		seen[key] = true
		start := runePos + utf8.RuneCountInString(text[bytePos:span[0]])
		end := start + length
		bytePos, runePos = span[1], end
		result = append(result, Candidate{Name: name, Abbreviation: token, Start: start, End: end, Before: string(chars[max(0, start-60):start]), After: string(chars[end:min(len(chars), end+60)])})
	}
	return result
}

type Hint struct {
	Ambiguous  bool   `json:"ambiguous,omitempty"`
	TermID     string `json:"termId,omitempty"`
	Version    int    `json:"version,omitempty"`
	Spelling   string `json:"spelling"`
	Name       string `json:"name,omitempty"`
	Definition string `json:"definition,omitempty"`
	Context    string `json:"context,omitempty"`
}

func SummaryHints(text string, terms []Term) []Hint {
	var hints []Hint
	seen := map[string]bool{}
	size := 0
	for _, m := range Matches(text, terms) {
		key := strings.ToLower(m.Text)
		if seen[key] {
			continue
		}
		seen[key] = true
		h := Hint{Spelling: m.Text, Ambiguous: len(m.Terms) > 1}
		if !h.Ambiguous {
			t := m.Terms[0]
			h.TermID = t.ID
			h.Version = t.Version
			h.Name = t.Name
			h.Definition = t.Definition
			h.Context = t.Context
		}
		encoded, _ := json.Marshal(h)
		if size+len(encoded)+1 > 3072 {
			continue
		}
		size += len(encoded) + 1
		hints = append(hints, h)
	}
	return hints
}
