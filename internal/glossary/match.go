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
type Candidate struct{ Name, Abbreviation string }

func word(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '_'
}

// Matches reports rune offsets in the original text. Choose the longest whole
// name at each position and keep every interpretation of an ambiguous name.
func Matches(text string, terms []Term) []Match {
	return compile(terms).matches(text)
}

type spelling struct {
	runes []rune
	terms []Term
}

type matcher map[rune][]*spelling

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

// Candidates only suggests expansions whose final words spell the abbreviation.
func Candidates(text string) []Candidate {
	var out []Candidate
	seen := map[Candidate]bool{}
	for _, span := range expansion.FindAllStringSubmatch(text, -1) {
		words := strings.Fields(span[1])
		abbr := []rune(span[2])
		if len(words) < len(abbr) {
			continue
		}
		words = words[len(words)-len(abbr):]
		var initials strings.Builder
		for _, w := range words {
			r, _ := utf8.DecodeRuneInString(w)
			initials.WriteRune(unicode.ToUpper(r))
		}
		candidate := Candidate{strings.Join(words, " "), span[2]}
		if initials.String() == span[2] && !seen[candidate] {
			out = append(out, candidate)
			seen[candidate] = true
		}
	}
	return out
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
