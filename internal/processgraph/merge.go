package processgraph

import (
	"slices"
	"strconv"
	"strings"
)

// mergeDraft combines already grounded batches. A node or condition with
// different wording stays distinct and each edge retains its original sources.
func mergeDraft(draft *Draft, batch Draft) {
	draft.Partial = draft.Partial || batch.Partial
	nodeKey := func(node Node) [2]string {
		return [2]string{node.Kind, strings.Join(strings.Fields(node.Label), " ")}
	}
	nodeIDs := make(map[[2]string]string, len(draft.Nodes))
	usedIDs := make(map[string]bool, len(draft.Nodes))
	for _, node := range draft.Nodes {
		nodeIDs[nodeKey(node)] = node.ID
		usedIDs[node.ID] = true
	}
	nextID := len(draft.Nodes) + 1
	batchIDs := make(map[string]string, len(batch.Nodes))
	for _, node := range batch.Nodes {
		key := nodeKey(node)
		id := nodeIDs[key]
		if id == "" {
			for id = strconv.Itoa(nextID); usedIDs[id]; id = strconv.Itoa(nextID) {
				nextID++
			}
			nextID++
			nodeIDs[key] = id
			draft.Nodes = append(draft.Nodes, Node{ID: id, Kind: node.Kind, Label: node.Label})
		}
		batchIDs[node.ID] = id
	}

	edgeKey := func(edge Edge) [4]string {
		return [4]string{edge.From, edge.To, edge.Kind, edge.Label}
	}
	edgeIndices := make(map[[4]string]int, len(draft.Edges))
	for i, edge := range draft.Edges {
		edgeIndices[edgeKey(edge)] = i
	}
	for _, edge := range batch.Edges {
		edge.From, edge.To = batchIDs[edge.From], batchIDs[edge.To]
		key := edgeKey(edge)
		i, exists := edgeIndices[key]
		if !exists {
			edgeIndices[key] = len(draft.Edges)
			draft.Edges = append(draft.Edges, edge)
			continue
		}
		for _, evidence := range edge.Evidence {
			if !slices.Contains(draft.Edges[i].Evidence, evidence) {
				draft.Edges[i].Evidence = append(draft.Edges[i].Evidence, evidence)
			}
		}
	}
}
