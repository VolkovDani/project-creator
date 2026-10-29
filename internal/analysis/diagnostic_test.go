package analysis

import (
	"context"
	"github.com/VolkovDani/project-creator/internal/rag"
	"reflect"
	"testing"
)

func TestMissingIndexesRequireCurrentVersionAndProfile(t *testing.T) {
	sources := []Source{{ID: "a", Version: 2}, {ID: "b", Version: 1}}
	cases := []struct {
		name   string
		states []rag.State
		want   []string
	}{
		{"one missing", []rag.State{{RequirementID: "a", Version: 2, Status: "ready", Profile: "e"}}, []string{"b"}},
		{"old version", []rag.State{{RequirementID: "a", Version: 1, Status: "ready", Profile: "e"}}, []string{"a", "b"}},
		{"old profile", []rag.State{{RequirementID: "a", Version: 2, Status: "ready", Profile: "old"}}, []string{"a", "b"}},
		{"failed", []rag.State{{RequirementID: "a", Version: 2, Status: "failed", Profile: "e"}}, []string{"a", "b"}},
		{"ready", []rag.State{{RequirementID: "a", Version: 2, Status: "ready", Profile: "e"}, {RequirementID: "b", Version: 1, Status: "ready", Profile: "e"}}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingIndexes(sources, tc.states, "e"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestTooFewRequirementsDoesNotNeedConfiguredProviders(t *testing.T) {
	for _, sources := range [][]Source{{}, {{ID: "a", Version: 1}}} {
		_, code, incomplete, d := (&Service{}).analyze(context.Background(), "a", "p", sources, false)
		if code != "" || !incomplete || d.ReasonCode != "TOO_FEW_REQUIREMENTS" {
			t.Fatalf("%s %v %+v", code, incomplete, d)
		}
	}
}
