package graph

import (
	"context"
	"encoding/json"
	"github.com/VolkovDani/project-creator/contracts"
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
		`mutation{replaceProjectAnalysis(projectId:"p",analysisId:"run",mode:"related",threshold:0.55,strategy:"wait"){id stage status}}`,
		`mutation{cancelProjectAnalysis(projectId:"p",analysisId:"run"){id status errorCode completedPairs pendingPairs runningPairs}}`,
		`mutation{startProjectAnalysis(projectId:"p",mode:"related",threshold:0.65){mode similarityThreshold selectionProfile possiblePairs notSelectedPairs duplicatePairs pairs{resultOrigin selectionReasons similarityScore}}}`,
		`query{projectAnalysis(projectId:"p"){id stage totalPairs processedPairs activePairId pairs{id position status left{id version text} right{id version text} verdict explanation errorCode}}}`,
		`query{analysisPairs(projectId:"p",analysisId:"r",limit:20,offset:20,status:"completed",verdict:"collision"){id reusedFrom checkedAt generatorProfile}}`,
		`query{analysisPair(projectId:"p",analysisId:"r",pairId:"pair"){explanationMatches{start end text terms{id}} detailsVersion isExactDuplicate left{id version text} right{id version text} selectionReasons similarityScore resultOrigin reusedFrom checkedAt generatorProfile layaStatus layaErrorCode preclassification layaCheckedAt}}`,
		`query{requirementIndexByIds(projectId:"p",requirementIds:["r"]){requirementId projectId version status profile errorCode}}`,
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

func TestAnalysisPairCarriesExactDuplicateAndOpaqueDetailsVersion(t *testing.T) {
	pair := analysis.PairResult{ID: "pair", Status: "completed", Verdict: "duplicate", ResultOrigin: "exact_text_v1", Explanation: "same canonical text"}
	first := analysisPair(pair)
	if !first.IsExactDuplicate || first.DetailsVersion == "" || first.DetailsVersion == "pair" {
		t.Fatalf("missing exact duplicate metadata: %+v", first)
	}
	pair.LayaStatus = "completed"
	second := analysisPair(pair)
	if first.DetailsVersion == second.DetailsVersion {
		t.Fatal("details version did not change when pair metadata changed")
	}
	pair.ResultOrigin = "model"
	if analysisPair(pair).IsExactDuplicate {
		t.Fatal("model duplicate was marked exact")
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

func TestReplacementStrategyValidationHasFieldMetadata(t *testing.T) {
	schema, err := graphql.ParseSchema(Schema, &Resolver{}, graphql.UseFieldResolvers())
	if err != nil {
		t.Fatal(err)
	}
	response := schema.Exec(WithBudget(context.Background()), `mutation{replaceProjectAnalysis(projectId:"p",analysisId:"r",strategy:"invalid"){id}}`, "", nil)
	if len(response.Errors) != 1 {
		t.Fatal(response)
	}
	raw, _ := json.Marshal(response.Errors[0].Extensions)
	var ext struct {
		FieldErrors []contracts.AnalysisFieldError `json:"fieldErrors"`
	}
	if err = json.Unmarshal(raw, &ext); err != nil || len(ext.FieldErrors) != 1 || ext.FieldErrors[0].Path[0] != "strategy" || ext.FieldErrors[0].Code != "INVALID_CHOICE" {
		t.Fatalf("metadata %s %v", raw, err)
	}
}
