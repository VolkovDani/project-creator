package processgraph

import (
	"slices"
	"strings"
	"unicode"

	"github.com/VolkovDani/project-creator/internal/glossary"
)

type conceptSource struct {
	id      string
	version int
}

type citedConcept struct {
	id, spelling     string
	start, statement int
}

type conceptProof struct {
	anchor    string
	matchText func(string) []glossary.Match
	sources   map[conceptSource][]citedConcept
}

func buildConceptProof(req Request) *conceptProof {
	if len(req.Glossary) == 0 {
		return nil
	}
	proof := &conceptProof{
		anchor: req.Anchor.ConceptID, matchText: glossary.MatchText(req.Glossary),
		sources: make(map[conceptSource][]citedConcept, len(req.Sources)),
	}
	for _, source := range req.Sources {
		key := conceptSource{source.ID, source.Version}
		runes := []rune(source.Text)
		statement, previousEnd := 0, 0
		for _, match := range proof.matchText(source.Text) {
			for position := previousEnd; position < match.Start; position++ {
				if isStatementEnd(runes, position) {
					statement++
				}
			}
			if len(match.Terms) == 1 {
				proof.sources[key] = append(proof.sources[key], citedConcept{
					id: match.Terms[0].ID, spelling: match.Text, start: source.Offset + match.Start,
					statement: statement,
				})
			}
			// A separator inside a matched alias is part of its spelling.
			previousEnd = match.End
		}
	}
	return proof
}

func isStatementEnd(text []rune, position int) bool {
	char := text[position]
	if strings.ContainsRune("\r\n;!?。！？", char) {
		return true
	}
	return char == '.' && position+1 < len(text) && unicode.IsSpace(text[position+1])
}

func (p *conceptProof) supports(from, to Node, evidence []Evidence) bool {
	if p == nil {
		return true
	}
	required := make(map[string]bool)
	for _, node := range []Node{from, to} {
		var role string
		for _, match := range p.matchText(node.Label) {
			if len(match.Terms) != 1 {
				return false
			}
			id := match.Terms[0].ID
			if node.Kind == "actor" {
				if role != "" && role != id {
					return false
				}
				role = id
			}
			crossRole := node.Kind == "actor" && p.anchor != "" && id != p.anchor
			required[id] = required[id] || crossRole
		}
	}
	if len(required) == 0 {
		return true
	}

	citations := make(map[conceptSource][]Evidence)
	for _, item := range evidence {
		key := conceptSource{item.RequirementID, item.Version}
		citations[key] = append(citations[key], item)
	}
	covered := make([]map[string]bool, 0, len(citations))
	for key, items := range citations {
		statements := map[int]map[string]bool{}
		for _, mention := range p.sources[key] {
			if !mention.isCited(items) {
				continue
			}
			if statements[mention.statement] == nil {
				statements[mention.statement] = make(map[string]bool)
			}
			statements[mention.statement][mention.id] = true
		}
		for _, concepts := range statements {
			covered = append(covered, concepts)
		}
	}
	for id, crossRole := range required {
		if !slices.ContainsFunc(covered, func(concepts map[string]bool) bool {
			return concepts[id] && (!crossRole || concepts[p.anchor])
		}) {
			return false
		}
	}
	return true
}

// Group already grounded roles by the confirmed identity of their whole label.
// Keep a single spelling as cited, and use the glossary name for grouped cards.
func (p *conceptProof) mergeActorCards(draft *Draft) {
	if p == nil || len(draft.Nodes) < 2 {
		return
	}
	groups := make(map[string]int)
	nodeIDs := make(map[string]string, len(draft.Nodes))
	nodes := make([]Node, 0, len(draft.Nodes))
	for _, node := range draft.Nodes {
		if node.Kind == "actor" {
			matches := p.matchText(node.Label)
			if len(matches) == 1 && matches[0].Text == node.Label && len(matches[0].Terms) == 1 {
				term := matches[0].Terms[0]
				if index, exists := groups[term.ID]; exists {
					nodes[index].Label = term.Name
					nodeIDs[node.ID] = nodes[index].ID
					continue
				}
				groups[term.ID] = len(nodes)
			}
		}
		nodeIDs[node.ID] = node.ID
		nodes = append(nodes, node)
	}
	if len(nodes) < len(draft.Nodes) {
		draft.Nodes = nodes
		draft.Edges = mergeEdges(nil, draft.Edges, nodeIDs)
	}
}

func (m citedConcept) isCited(evidence []Evidence) bool {
	// Use the complete original occurrence. Neighboring quotes can cover an
	// alias split at an abbreviation, even if trimming removed its spaces.
	for i, char := range []rune(m.spelling) {
		if unicode.IsSpace(char) {
			continue
		}
		position := m.start + i
		if !slices.ContainsFunc(evidence, func(item Evidence) bool {
			return item.Start <= position && position < item.End
		}) {
			return false
		}
	}
	return true
}
