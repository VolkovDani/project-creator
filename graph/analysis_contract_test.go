package graph

import (
	"testing"

	graphql "github.com/graph-gophers/graphql-go"
)

func TestProgressiveAnalysisContract(t *testing.T) {
	schema, err := graphql.ParseSchema(Schema, &Resolver{}, graphql.UseFieldResolvers())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`query{projectAnalysis(projectId:"p"){id stage totalPairs processedPairs activePairId pairs{id position status left{id version text} right{id version text} verdict explanation errorCode}}}`,
		`mutation{retryProjectAnalysis(projectId:"p",analysisId:"run"){id status stage totalPairs processedPairs pairs{id status}}}`,
	} {
		if errors := schema.Validate(query); len(errors) > 0 {
			t.Errorf("progressive analysis operation rejected: %v", errors)
		}
	}
}
