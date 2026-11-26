// Package processgraph builds source-grounded drafts of product processes.
package processgraph

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxSources   = 12
	MaxNodes     = 20
	MaxEdges     = 8
	MaxCitations = 3
	MaxLabel     = 120
	MaxQuote     = 600
)

type Anchor struct {
	Name       string `json:"name"`
	Definition string `json:"definition,omitempty"`
	Context    string `json:"context,omitempty"`
}

type Source struct {
	ID, Text string
	Version  int
}

type Request struct {
	Anchor  Anchor
	Sources []Source
}

type Generator interface {
	GenerateProcessGraph(context.Context, Request) (Suggestion, error)
}

type Suggestion struct {
	Nodes       []Node
	Edges       []SuggestedEdge
	UsedSources int
	Partial     bool
}

type Node struct {
	ID, Kind, Label string
}

type SuggestedEdge struct {
	From, To, Kind, Label string
	Evidence              []SuggestedEvidence
}

type SuggestedEvidence struct {
	Source int
	Quote  string
	Start  *int // Optional offset of the exact quoted occurrence.
}

type Draft struct {
	Nodes       []Node
	Edges       []Edge
	SourceCount int
	Partial     bool
}

type Edge struct {
	From, To, Kind, Label string
	Evidence              []Evidence
}

type Evidence struct {
	RequirementID, Quote string
	Version, Start, End  int // Start and End are Unicode code point offsets.
}

var nodeKinds = []string{"actor", "action", "condition", "event", "state", "object", "rule"}
var edgeKinds = []string{"performs", "follows", "triggers", "requires", "permits", "prohibits", "leads_to", "constrains", "needs"}
var nodeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// NodeKinds and EdgeKinds are shared by the validator and the generation schema.
func NodeKinds() []string { return slices.Clone(nodeKinds) }
func EdgeKinds() []string { return slices.Clone(edgeKinds) }

// Build verifies citations against the exact source snapshots used by the
// generator. The relationship and labels remain suggestions for human review.
func Build(ctx context.Context, g Generator, req Request) (Draft, error) {
	if g == nil || len(req.Sources) == 0 || len(req.Sources) > MaxSources {
		return Draft{}, errors.New("invalid process graph request")
	}
	suggestion, err := g.GenerateProcessGraph(ctx, req)
	var draft Draft
	if err == nil {
		draft, err = groundDraft(suggestion, req)
		if len(draft.Edges) > 0 {
			return draft, nil
		}
	}
	if ctx.Err() != nil {
		return Draft{}, ctx.Err()
	}
	// A provider may return nothing usable for an explicitly written rule.
	// Keep its literal condition and result without guessing a transition.
	literal, literalErr := groundDraft(literalRelations(req.Sources), req)
	if literalErr == nil && len(literal.Edges) > 0 {
		return literal, nil
	}
	return draft, err
}

func groundDraft(suggestion Suggestion, req Request) (Draft, error) {
	if suggestion.UsedSources < 1 || suggestion.UsedSources > len(req.Sources) ||
		len(suggestion.Nodes) > MaxNodes || len(suggestion.Edges) > MaxEdges {
		return Draft{}, errors.New("invalid process graph output")
	}

	draft := Draft{
		SourceCount: suggestion.UsedSources,
		Partial:     suggestion.Partial || suggestion.UsedSources < len(req.Sources) || len(suggestion.Edges) == MaxEdges,
	}
	sources := req.Sources[:suggestion.UsedSources]
	nodes := validatedNodes(suggestion.Nodes)
	used := map[string]bool{}
	edges := map[[4]string]bool{}
	for _, edge := range suggestion.Edges {
		from, hasFrom := nodes[edge.From]
		to, hasTo := nodes[edge.To]
		if !hasFrom || !hasTo || !validEndpoints(edge.Kind, from.Kind, to.Kind) || !validLabel(edge.Label, true) {
			continue
		}
		key := [4]string{edge.From, edge.To, edge.Kind, edge.Label}
		if edges[key] {
			continue
		}
		citations := groundedEvidence(edge.Evidence, sources, req.Anchor.Name)
		if len(citations) == 0 || !supportsLabel(edge.Label, citations) {
			continue
		}
		edges[key] = true
		used[edge.From], used[edge.To] = true, true
		draft.Edges = append(draft.Edges, Edge{
			From: edge.From, To: edge.To, Kind: edge.Kind, Label: edge.Label, Evidence: citations,
		})
	}
	for _, node := range suggestion.Nodes {
		if used[node.ID] {
			draft.Nodes = append(draft.Nodes, node)
		}
	}
	return draft, nil
}

func supportsLabel(label string, citations []Evidence) bool {
	if label == "" {
		return true
	}
	normalize := func(text string) string {
		return strings.ToLower(strings.Join(strings.Fields(text), " "))
	}
	label = normalize(label)
	return slices.ContainsFunc(citations, func(evidence Evidence) bool {
		return strings.Contains(normalize(evidence.Quote), label)
	})
}

func validEndpoints(edge, from, to string) bool {
	switch edge {
	case "performs":
		return from == "actor" && to == "action"
	case "needs":
		return from == "actor" && (to == "action" || to == "event" || to == "state")
	case "follows":
		return from == "action" && to == "action"
	case "triggers":
		return (from == "action" || from == "event" || from == "state") && to == "action"
	case "permits", "prohibits":
		return (from == "actor" || from == "rule") && to == "action"
	case "requires":
		return from == "action"
	case "leads_to":
		return from != "actor" && from != "object" && from != "rule" && to != "actor" && to != "rule"
	case "constrains":
		return (from == "rule" || from == "condition") && (to == "action" || to == "object")
	default:
		return false
	}
}

func validatedNodes(input []Node) map[string]Node {
	counts := make(map[string]int, len(input))
	for _, node := range input {
		counts[node.ID]++
	}
	nodes := make(map[string]Node, len(input))
	for _, node := range input {
		if counts[node.ID] == 1 && nodeID.MatchString(node.ID) &&
			slices.Contains(nodeKinds, node.Kind) && validLabel(node.Label, false) {
			nodes[node.ID] = node
		}
	}
	return nodes
}

func validLabel(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		utf8.RuneCountInString(value) <= MaxLabel && strings.IndexFunc(value, unicode.IsControl) < 0
}

func groundedEvidence(citations []SuggestedEvidence, sources []Source, anchor string) []Evidence {
	if len(citations) == 0 || len(citations) > MaxCitations {
		return nil
	}
	var evidence []Evidence
	for _, citation := range citations {
		quote := citation.Quote
		if citation.Source < 1 || citation.Source > len(sources) ||
			strings.TrimSpace(quote) != quote || quote == "" || !utf8.ValidString(quote) ||
			utf8.RuneCountInString(quote) > MaxQuote ||
			strings.EqualFold(strings.Trim(quote, ".,:!?«»\""), anchor) {
			return nil
		}
		source := sources[citation.Source-1]
		length := utf8.RuneCountInString(quote)
		var start int
		if citation.Start == nil {
			pos := strings.Index(source.Text, quote)
			if pos < 0 {
				return nil
			}
			start = utf8.RuneCountInString(source.Text[:pos])
		} else {
			start = *citation.Start
			raw := []rune(source.Text)
			if start < 0 || start > len(raw) || length > len(raw)-start || string(raw[start:start+length]) != quote {
				return nil
			}
		}
		item := Evidence{
			RequirementID: source.ID, Version: source.Version, Quote: quote,
			Start: start, End: start + length,
		}
		if !slices.Contains(evidence, item) {
			evidence = append(evidence, item)
		}
	}
	return evidence
}
