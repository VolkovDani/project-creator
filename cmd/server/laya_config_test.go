package main

import (
	"strings"
	"testing"
)

func TestLayaClassifierConfigurationIsOptionalAndPinned(t *testing.T) {
	for _, tc := range []struct {
		name             string
		env              map[string]string
		wantErr, wantNil bool
	}{
		{"disabled", nil, false, true},
		{"incomplete", map[string]string{"PROJECT_CREATOR_LAYA_MODEL": "laya:322m-multilingual-bf16"}, true, false},
		{"configured", map[string]string{"PROJECT_CREATOR_LAYA_MODEL": "laya:322m-multilingual-bf16", "PROJECT_CREATOR_LAYA_DIGEST": strings.Repeat("a", 64), "PROJECT_CREATOR_LAYA_URL": "http://ollama-laya:11434"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			classifier, err := configuredClassifier(func(key string) string { return tc.env[key] })
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if err == nil && (classifier == nil) != tc.wantNil {
				t.Fatalf("classifier=%v", classifier)
			}
		})
	}
}
