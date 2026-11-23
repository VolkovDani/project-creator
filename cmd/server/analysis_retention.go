package main

import (
	"fmt"
	"strconv"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

func configuredAnalysisHistoryRetentionDays(getenv func(string) string) (int, error) {
	const key = "ANALYSIS_HISTORY_RETENTION_DAYS"
	raw := getenv(key)
	if raw == "" {
		return analysis.DefaultRetentionDays, nil
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > analysis.MaxRetentionDays {
		return 0, fmt.Errorf("%s must be a whole number between 1 and %d", key, analysis.MaxRetentionDays)
	}
	return days, nil
}
