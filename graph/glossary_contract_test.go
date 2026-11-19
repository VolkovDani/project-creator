package graph

import (
	"testing"

	graphql "github.com/graph-gophers/graphql-go"
)

func TestGlossaryTermsByIdsContract(t *testing.T) {
	schema, err := graphql.ParseSchema(Schema, &Resolver{}, graphql.UseFieldResolvers())
	if err != nil {
		t.Fatal(err)
	}
	query := `query($projectId: ID!, $ids: [ID!]!) {
		glossaryTermsByIds(projectId: $projectId, ids: $ids) {
			id name definition context version variants { text kind }
			sourceId sourceVersion createdAt updatedAt
		}
	}`
	if errs := schema.ValidateWithVariables(query, map[string]any{"projectId": "p", "ids": []any{"t"}}); len(errs) != 0 {
		t.Fatalf("glossary batch operation rejected: %v", errs)
	}
}
