package assistantmcp

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type similarPairsInput struct {
	pageInput
	Threshold float64 `json:"threshold" jsonschema:"Minimum cosine similarity from 0 to 1, inclusive. Use the same value when continuing."`
}

type similarPair struct {
	Left            requirement    `json:"left"`
	Right           requirement    `json:"right"`
	SimilarityScore float64        `json:"similarityScore"`
	GlossaryContext []glossaryHint `json:"glossaryContext"`
}

type glossaryHint struct {
	TermID     *string `json:"termId,omitempty"`
	Version    *int32  `json:"version,omitempty"`
	Spelling   string  `json:"spelling"`
	Name       *string `json:"name,omitempty"`
	Definition *string `json:"definition,omitempty"`
	Context    *string `json:"context,omitempty"`
	Ambiguous  bool    `json:"ambiguous"`
}

type similarPairsPage struct {
	Items               []similarPair `json:"items"`
	PageInfo            pageInfo      `json:"pageInfo"`
	Status              string        `json:"status"`
	EmbeddingProfile    string        `json:"embeddingProfile"`
	MissingRequirements int           `json:"missingRequirements"`
}

func (b backend) similarPairs(ctx context.Context, _ *mcp.CallToolRequest, in similarPairsInput) (*mcp.CallToolResult, similarPairsPage, error) {
	variables, err := in.variables(b.cfg.ProjectID)
	if err != nil {
		return nil, similarPairsPage{}, err
	}
	if !contracts.AnalysisConstraints().ValidThreshold(in.Threshold) {
		return nil, similarPairsPage{}, errors.New("threshold must be between 0 and 1")
	}
	variables["threshold"] = in.Threshold
	var result struct{ SimilarRequirementPairs similarPairsPage }
	err = b.query(ctx, `query MCPSimilarRequirementPairs($projectId: ID!, $threshold: Float!, $first: Int!, $after: String) {
		similarRequirementPairs(projectId: $projectId, threshold: $threshold, first: $first, after: $after) {
			items {
				left { id text version }
				right { id text version }
				similarityScore
				glossaryContext { termId version spelling name definition context ambiguous }
			}
			pageInfo { endCursor hasNextPage }
			status embeddingProfile missingRequirements
		}
	}`, variables, &result)
	if err != nil {
		return nil, similarPairsPage{}, err
	}
	page := result.SimilarRequirementPairs
	return &mcp.CallToolResult{IsError: page.Status != "ready"}, page, nil
}
