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

func TestSuggestGlossaryDefinitionContract(t *testing.T) {
	schema, err := graphql.ParseSchema(Schema, &Resolver{}, graphql.UseFieldResolvers())
	if err != nil {
		t.Fatal(err)
	}
	query := `mutation($input: SuggestGlossaryDefinitionInput!) {
		suggestGlossaryDefinition(input: $input) { definition status fieldErrors { path code messageKey params { key value } } }
	}`
	input := map[string]any{"projectId": "p", "sourceId": "r", "sourceVersion": int32(1), "start": int32(2), "end": int32(5), "name": "ЭЦП"}
	if errs := schema.ValidateWithVariables(query, map[string]any{"input": input}); len(errs) != 0 {
		t.Fatalf("glossary suggestion rejected: %v", errs)
	}
}
