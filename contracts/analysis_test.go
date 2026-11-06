package contracts

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestSharedAnalysisSelectionFixtures(t *testing.T) {
	raw, err := os.ReadFile("analysis.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name  string
		Input struct {
			Mode      *string
			Threshold *float64
		}
		Normalized AnalysisSelectionInput
		Errors     []AnalysisFieldError
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			normalized, errors := AnalysisSelection(fixture.Input.Mode, fixture.Input.Threshold)
			if !reflect.DeepEqual(normalized, fixture.Normalized) || !reflect.DeepEqual(errors, fixture.Errors) {
				t.Fatalf("got %+v %+v want %+v %+v", normalized, errors, fixture.Normalized, fixture.Errors)
			}
		})
	}
}
func TestAnalysisRulesRejectNonFiniteThresholdAndReturnFreshModes(t *testing.T) {
	mode, threshold := AnalysisDefaults()
	if !ValidateAnalysisSelection(mode, threshold) {
		t.Fatal("invalid defaults")
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if ValidateAnalysisSelection(mode, value) {
			t.Fatal("non-finite threshold accepted")
		}
	}
	rules := AnalysisConstraints()
	rules.Modes[0] = "changed"
	if AnalysisConstraints().Modes[0] == "changed" {
		t.Fatal("mutable global rules")
	}
	if ValidateAnalysisPage(0, 0) || ValidateAnalysisPage(101, 0) || ValidateAnalysisPage(1, -1) || !ValidateAnalysisPage(100, 0) {
		t.Fatal("invalid page constraints")
	}
}

func TestSharedAnalysisReplacementStrategyFixtures(t *testing.T) {
	raw, err := os.ReadFile("analysis-replacement.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name     string
		Strategy string
		Errors   []AnalysisFieldError
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			got := AnalysisReplacementStrategy(fixture.Strategy)
			if !reflect.DeepEqual(got, fixture.Errors) {
				t.Fatalf("got %+v want %+v", got, fixture.Errors)
			}
		})
	}
}
