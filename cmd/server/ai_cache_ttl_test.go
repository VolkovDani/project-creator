package main

import (
	"testing"
	"time"
)

func TestConfiguredAICacheTTL(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
		valid bool
	}{
		{"", 0, true},
		{"0", 0, true},
		{"0s", 0, true},
		{"24h", 24 * time.Hour, true},
		{"-1h", 0, false},
		{"1 day", 0, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, err := configuredAICacheTTL(func(key string) string {
				if key != "PROJECT_CREATOR_AI_CACHE_TTL" {
					t.Fatalf("unexpected configuration key %s", key)
				}
				return tc.value
			})
			if got != tc.want || (err == nil) != tc.valid {
				t.Fatalf("got %s, %v", got, err)
			}
		})
	}
}
