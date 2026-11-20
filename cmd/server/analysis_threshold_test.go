package main

import (
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/contracts"
)

func TestConfiguredAnalysisDefaultThreshold(t *testing.T) {
	const key = "PROJECT_CREATOR_ANALYSIS_DEFAULT_THRESHOLD"
	for _, test := range []struct {
		raw   string
		want  float64
		valid bool
	}{
		{"", contracts.AnalysisConstraints().DefaultThreshold, true}, {"0.82", 0.82, true},
		{"0", 0, true}, {"1", 1, true},
		{"invalid", 0, false}, {"NaN", 0, false}, {"+Inf", 0, false}, {"-Inf", 0, false},
		{"-0.01", 0, false}, {"1.01", 0, false}, {" ", 0, false},
	} {
		t.Run(test.raw, func(t *testing.T) {
			got, err := configuredAnalysisDefaultThreshold(func(k string) string {
				if k != key {
					t.Fatalf("unexpected key %q", k)
				}
				return test.raw
			})
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("got %v, %v; want %v", got, err, test.want)
				}
			} else if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("expected configuration error naming %s, got %v", key, err)
			}
		})
	}
}
