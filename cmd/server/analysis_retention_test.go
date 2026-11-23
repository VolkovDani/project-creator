package main

import (
	"strings"
	"testing"
)

func TestConfiguredAnalysisHistoryRetentionDays(t *testing.T) {
	const key = "ANALYSIS_HISTORY_RETENTION_DAYS"
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"", 30}, {"1", 1}, {"45", 45}, {"106751", 106751},
		{"0", 0}, {"-1", 0}, {"2.5", 0}, {"106752", 0}, {"99999999999999999999", 0}, {"garbage", 0},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, err := configuredAnalysisHistoryRetentionDays(func(k string) string {
				if k != key {
					t.Fatalf("unexpected environment key %q", k)
				}
				return tc.value
			})
			if tc.want == 0 {
				if err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("expected error naming %s, got %v", key, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("retention = %d, %v; want %d days", got, err, tc.want)
			}
		})
	}
}
