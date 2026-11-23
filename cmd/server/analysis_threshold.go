package main

import (
	"fmt"
	"strconv"

	"github.com/VolkovDani/project-creator/contracts"
)

func configuredAnalysisDefaultThreshold(getenv func(string) string) (float64, error) {
	const key = "PROJECT_CREATOR_ANALYSIS_DEFAULT_THRESHOLD"
	rules := contracts.AnalysisConstraints()
	raw := getenv(key)
	if raw == "" {
		return rules.DefaultThreshold, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || !rules.ValidThreshold(v) {
		return 0, fmt.Errorf("%s must be a finite number between %g and %g", key, rules.MinThreshold, rules.MaxThreshold)
	}
	return v, nil
}
