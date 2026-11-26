package analysis

import (
	"encoding/json"
	"strings"

	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

const maxProcessGraphGlossaryBytes = 512

type processGraphConcept struct {
	Spelling  string `json:"spelling"`
	Name      string `json:"name,omitempty"`
	Ambiguous bool   `json:"ambiguous,omitempty"`
}

type processGraphGlossaryMatcher func(processgraph.Source) []glossary.Match

func processGraphGlossaryMatches(req processgraph.Request) processGraphGlossaryMatcher {
	if len(req.Glossary) == 0 {
		return nil
	}
	sources := req.OriginalSources
	if len(sources) == 0 {
		sources = req.Sources
	}
	type sourceKey struct {
		id      string
		version int
	}
	texts := make(map[sourceKey]string, len(sources))
	for _, source := range sources {
		if source.Offset == 0 {
			texts[sourceKey{source.ID, source.Version}] = source.Text
		}
	}
	matchText := glossary.MatchText(req.Glossary)
	matches := make(map[sourceKey][]glossary.Match, len(texts))
	return func(source processgraph.Source) []glossary.Match {
		key := sourceKey{source.ID, source.Version}
		if _, found := matches[key]; !found {
			matches[key] = matchText(texts[key])
		}
		return matches[key]
	}
}

func appendProcessGraphGlossary(concepts []processGraphConcept, matches []glossary.Match, start, end int) []processGraphConcept {
	seen := make(map[string]bool, len(concepts))
	size := 2 // Array brackets.
	if len(concepts) > 0 {
		encoded, _ := json.Marshal(concepts)
		size = len(encoded)
	}
	for _, concept := range concepts {
		seen[strings.ToLower(concept.Spelling)] = true
	}
	for _, match := range matches {
		if match.Start < start {
			continue
		}
		if match.End > end {
			break
		}
		key := strings.ToLower(match.Text)
		if seen[key] {
			continue
		}
		seen[key] = true
		concept := processGraphConcept{Spelling: match.Text, Ambiguous: len(match.Terms) > 1}
		if !concept.Ambiguous {
			concept.Name = match.Terms[0].Name
		}
		encoded, _ := json.Marshal(concept)
		added := len(encoded)
		if len(concepts) > 0 {
			added++ // Array comma.
		}
		if size+added <= maxProcessGraphGlossaryBytes {
			concepts = append(concepts, concept)
			size += added
		}
	}
	return concepts
}
