package graph

import (
	"context"
	"encoding/json"
	"github.com/VolkovDani/project-creator/internal/analysis"
	"testing"

	graphql "github.com/graph-gophers/graphql-go"
)

func TestProgressiveAnalysisContract(t *testing.T) {
	schema, err := graphql.ParseSchema(Schema, &Resolver{}, graphql.UseFieldResolvers())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`mutation{startProjectAnalysis(projectId:"p",mode:"related",threshold:0.65){mode similarityThreshold selectionProfile possiblePairs notSelectedPairs duplicatePairs pairs{resultOrigin selectionReasons similarityScore}}}`,
		`query{projectAnalysis(projectId:"p"){id stage totalPairs processedPairs activePairId pairs{id position status left{id version text} right{id version text} verdict explanation errorCode}}}`,
		`query{analysisPairs(projectId:"p",analysisId:"r",limit:20,offset:20,status:"completed",verdict:"collision"){id reusedFrom checkedAt generatorProfile}}`,
		`query{analysisHistory(projectId:"p",limit:20,offset:0){id reusedPairs newPairs completedPairs pendingPairs runningPairs failedPairs collisionPairs compatiblePairs insufficientPairs generatorProvenance}}`,
		`query{projectAnalysis(projectId:"p",analysisId:"r"){id}}`,
		`mutation{retryProjectAnalysis(projectId:"p",analysisId:"run"){id status stage totalPairs processedPairs pairs{id status}}}`,
	} {
		if errors := schema.Validate(query); len(errors) > 0 {
			t.Errorf("progressive analysis operation rejected: %v", errors)
		}
	}
}

func TestAnalysisAggregateMappingIsIndependentOfPage(t *testing.T) {
	total := 4950
	run := analysis.Run{TotalPairs: &total, CompletedPairs: 45, ReusedPairs: 30, NewPairs: 15, PendingPairs: 4905, CompatiblePairs: 45, GeneratorProvenance: `{"adapter":"fake"}`, Pairs: []analysis.PairResult{{ID: "one", Status: "completed", Verdict: "compatible", ReusedFrom: "original", CheckedAt: "time", GeneratorProfile: "profile"}}}
	value := projectAnalysis(run)
	if *value.TotalPairs != 4950 || value.CompletedPairs != 45 || value.CompatiblePairs != 45 || value.PendingPairs != 4905 || value.ReusedPairs != 30 || value.NewPairs != 15 {
		t.Fatalf("page replaced aggregate coverage: %+v", value)
	}
	if len(value.Pairs) != 1 || value.Pairs[0].ReusedFrom != "original" || value.Pairs[0].CheckedAt != "time" || value.Pairs[0].GeneratorProfile != "profile" || value.GeneratorProvenance != run.GeneratorProvenance {
		t.Fatalf("lost provenance: %+v", value)
	}
}

func TestAnalysisOptionsDefaultAndValidation(t *testing.T) {
	options, err := analysisOptions(nil, nil)
	if err != nil || options.Mode != "related" || options.Threshold != .65 {
		t.Fatalf("defaults: %+v %v", options, err)
	}
	mode := "all"
	zero := 0.0
	options, err = analysisOptions(&mode, &zero)
	if err != nil || options.Mode != "all" || options.Threshold != 0 {
		t.Fatalf("explicit zero: %+v %v", options, err)
	}
	invalid := "unknown"
	if _, err = analysisOptions(&invalid, nil); err == nil {
		t.Fatal("unknown mode accepted")
	}
	for _, threshold := range []float64{-0.1, 1.1} {
		if _, err = analysisOptions(nil, &threshold); err == nil {
			t.Fatal("invalid threshold accepted")
		}
	}
}

func TestAnalysisSelectionValidationHasStableFieldMetadata(t *testing.T) {
	schema, err := graphql.ParseSchema(Schema, &Resolver{}, graphql.UseFieldResolvers())
	if err != nil {
		t.Fatal(err)
	}
	response := schema.Exec(WithBudget(context.Background()), `mutation{startProjectAnalysis(projectId:"p",mode:"unknown",threshold:2){id}}`, "", nil)
	if len(response.Errors) != 1 {
		t.Fatalf("validation response: %+v", response)
	}
	extensions := response.Errors[0].Extensions
	raw, err := json.Marshal(extensions)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		FieldErrors []struct {
			Path             []string
			Code, MessageKey string
			Params           map[string]float64
		}
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.FieldErrors) != 2 || got.FieldErrors[0].Path[0] != "mode" || got.FieldErrors[0].Code != "INVALID_CHOICE" || got.FieldErrors[1].Path[0] != "threshold" || got.FieldErrors[1].Code != "RANGE" || got.FieldErrors[1].MessageKey != "analysis.validation.threshold" || got.FieldErrors[1].Params["max"] != 1 {
		t.Fatalf("missing structured validation: %s", raw)
	}
}
