package processgraph

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const clauseStart = `(?im)(?:^\s*(?:[-*]\s+|\d+[.)]\s+)?|[.!?]\s+)`

type literalPattern struct {
	clause           *regexp.Regexp
	fromKind, toKind string
	relationship     string
}

// Recover directly stated clauses without inferring any ordering between them.
var literalPatterns = []literalPattern{
	{
		clause:   regexp.MustCompile(clauseStart + `((?:если|if)[\p{Zs}\t]+([^,.!?;\r\n]+),[\p{Zs}\t]+(?:(?:то|then)[\p{Zs}\t]+)?([^.!?;\r\n]+))`),
		fromKind: "condition", toKind: "event", relationship: "leads_to",
	},
	{
		clause:   regexp.MustCompile(clauseStart + `(([^,.!?;\r\n]+?)[\p{Zs}\t]+(?:запрещено|не может|не вправе|не имеет права|cannot|may not|must not|is prohibited from)[\p{Zs}\t]+([^.!?;\r\n]+))`),
		fromKind: "actor", toKind: "action", relationship: "prohibits",
	},
}

func literalRelations(sources []Source) Suggestion {
	result := Suggestion{UsedSources: len(sources), Partial: true}
	nodeIDs := map[[2]string]string{}
	node := func(kind, label string) string {
		key := [2]string{kind, strings.ToLower(label)}
		id, found := nodeIDs[key]
		if !found {
			id = strconv.Itoa(len(result.Nodes) + 1)
			nodeIDs[key] = id
			result.Nodes = append(result.Nodes, Node{ID: id, Kind: kind, Label: label})
		}
		return id
	}
	for i, source := range sources {
		for _, pattern := range literalPatterns {
			for _, match := range pattern.clause.FindAllStringSubmatchIndex(source.Text, -1) {
				left := strings.TrimSpace(source.Text[match[4]:match[5]])
				right := strings.TrimSpace(source.Text[match[6]:match[7]])
				if !validLabel(left, false) || !validLabel(right, false) {
					continue
				}
				start := utf8.RuneCountInString(source.Text[:match[2]])
				result.Edges = append(result.Edges, SuggestedEdge{
					From: node(pattern.fromKind, left), To: node(pattern.toKind, right), Kind: pattern.relationship,
					Evidence: []SuggestedEvidence{{Source: i + 1, Start: &start, Quote: strings.TrimSpace(source.Text[match[2]:match[3]])}},
				})
				if len(result.Edges) == MaxEdges {
					return result
				}
			}
		}
	}
	return result
}
