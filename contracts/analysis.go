package contracts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

//go:embed analysis.json
var analysisJSON []byte

type AnalysisConfig struct {
	Version          int      `json:"version"`
	Modes            []string `json:"modes"`
	DefaultMode      string   `json:"defaultMode"`
	DefaultThreshold float64  `json:"defaultThreshold"`
	MinThreshold     float64  `json:"minThreshold"`
	MaxThreshold     float64  `json:"maxThreshold"`
	PageMin          int      `json:"pageMin"`
	PageMax          int      `json:"pageMax"`
	OffsetMin        int      `json:"offsetMin"`
}

type AnalysisSelectionInput struct {
	Mode      string  `json:"mode"`
	Threshold float64 `json:"threshold"`
}

type AnalysisFieldError struct {
	Path       []string           `json:"path"`
	Code       string             `json:"code"`
	MessageKey string             `json:"messageKey"`
	Params     map[string]float64 `json:"params"`
}

// AnalysisConstraints returns a fresh value, including its modes slice.
func AnalysisConstraints() AnalysisConfig {
	var rules AnalysisConfig
	decoder := json.NewDecoder(bytes.NewReader(analysisJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rules); err != nil {
		panic(fmt.Sprintf("invalid analysis contract: %v", err))
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		panic("invalid analysis contract: trailing JSON")
	}
	if rules.Version != 1 || len(rules.Modes) == 0 || rules.MinThreshold > rules.MaxThreshold || rules.DefaultThreshold < rules.MinThreshold || rules.DefaultThreshold > rules.MaxThreshold || rules.PageMin < 1 || rules.PageMax < rules.PageMin || rules.OffsetMin < 0 {
		panic("unsupported analysis contract")
	}
	found := false
	seen := map[string]bool{}
	for _, mode := range rules.Modes {
		if (mode != "related" && mode != "all") || seen[mode] {
			panic("unsupported analysis mode")
		}
		seen[mode] = true
		found = found || mode == rules.DefaultMode
	}
	if !found {
		panic("invalid default analysis mode")
	}
	return rules
}
func AnalysisDefaults() (string, float64) {
	rules := AnalysisConstraints()
	return rules.DefaultMode, rules.DefaultThreshold
}
func AnalysisSelection(mode *string, threshold *float64) (AnalysisSelectionInput, []AnalysisFieldError) {
	rules := AnalysisConstraints()
	out := AnalysisSelectionInput{rules.DefaultMode, rules.DefaultThreshold}
	if mode != nil {
		out.Mode = *mode
	}
	if threshold != nil {
		out.Threshold = *threshold
	}
	errors := []AnalysisFieldError{}
	found := false
	for _, allowed := range rules.Modes {
		found = found || out.Mode == allowed
	}
	if !found {
		errors = append(errors, AnalysisFieldError{[]string{"mode"}, "INVALID_CHOICE", "analysis.validation.mode", map[string]float64{}})
	}
	if math.IsNaN(out.Threshold) || math.IsInf(out.Threshold, 0) || out.Threshold < rules.MinThreshold || out.Threshold > rules.MaxThreshold {
		errors = append(errors, AnalysisFieldError{[]string{"threshold"}, "RANGE", "analysis.validation.threshold", map[string]float64{"min": rules.MinThreshold, "max": rules.MaxThreshold}})
	}
	return out, errors
}
func ValidateAnalysisSelection(mode string, threshold float64) bool {
	_, errors := AnalysisSelection(&mode, &threshold)
	return len(errors) == 0
}
func ValidateAnalysisPage(limit, offset int) bool {
	rules := AnalysisConstraints()
	return limit >= rules.PageMin && limit <= rules.PageMax && offset >= rules.OffsetMin
}
