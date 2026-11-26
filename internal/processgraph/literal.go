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
	{
		clause: regexp.MustCompile(clauseStart + `((?:я[\p{Zs}	,]+)?как[\p{Zs}	]+([^,
]+?)[\p{Zs}	,]+(?:я[\p{Zs}	]+)?(?:хочу|долж(?:ен|на|ны)[\p{Zs}	]+иметь[\p{Zs}	]+возможность)[\p{Zs}	,]+([^.!?;
]+))`),
		fromKind: "actor", toKind: "action", relationship: "needs",
	},
	{
		clause: regexp.MustCompile(clauseStart + `(as[\p{Zs}	]+(?:an?[\p{Zs}	]+)?([^,
]+?)[\p{Zs}	,]+(?:i|we)[\p{Zs}	]+(?:want|need)[\p{Zs}	]+to[\p{Zs}	]+([^.!?;
]+))`),
		fromKind: "actor", toKind: "action", relationship: "needs",
	},
}

func literalRelations(sources []Source) Suggestion {
	result := Suggestion{Partial: true}
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
		result.UsedSources = i + 1
		for _, pattern := range literalPatterns {
			for _, match := range pattern.clause.FindAllStringSubmatchIndex(source.Text, -1) {
				left := strings.TrimSpace(source.Text[match[4]:match[5]])
				right := strings.TrimSpace(source.Text[match[6]:match[7]])
				if pattern.relationship == "needs" {
					right = storyAction(right)
				}
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

var storyActionStarter = regexp.MustCompile(`(?i)^чтобы[\p{Zs}	]+`)
var storyPurpose = regexp.MustCompile(`(?i)(?:,[\p{Zs}	]*(?:для[\p{Zs}	]+того[\p{Zs}	,]+)?чтобы|[\p{Zs}	]+(?:so[\p{Zs}	]+that|in[\p{Zs}	]+order[\p{Zs}	]+to))[\p{Zs}	]+`)

func storyAction(text string) string {
	if lead := storyActionStarter.FindStringIndex(text); lead != nil {
		text = text[lead[1]:]
	}
	if purpose := storyPurpose.FindStringIndex(text); purpose != nil {
		text = text[:purpose[0]]
	}
	return strings.TrimSpace(text)
}
