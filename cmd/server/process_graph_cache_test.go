package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestClearProcessGraphCacheRejectsInvalidScopeBeforeDatabase(t *testing.T) {
	for _, args := range [][]string{
		{"--project-id", "", "--yes"},
		{"--project-id", " ", "--yes"},
		{"--yes", "unexpected"},
	} {
		var output bytes.Buffer
		err := runClearProcessGraphCache(context.Background(), args, func(string) string {
			t.Fatal("invalid arguments reached database configuration")
			return ""
		}, &output)
		if err == nil || output.Len() != 0 {
			t.Fatalf("args %q: output %q, error %v", args, &output, err)
		}
	}
}

func TestClearProcessGraphCacheDoesNotExposeDatabaseURL(t *testing.T) {
	var output bytes.Buffer
	const secret = "password-that-must-not-be-printed"
	err := runClearProcessGraphCache(context.Background(), nil, func(key string) string {
		if key != "PROJECT_CREATOR_DATABASE_URL" {
			t.Fatalf("unexpected environment lookup %q", key)
		}
		return "invalid://user:" + secret + "@localhost/db"
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "database") || strings.Contains(err.Error(), secret) || output.Len() != 0 {
		t.Fatalf("output %q, error %v", &output, err)
	}
}
