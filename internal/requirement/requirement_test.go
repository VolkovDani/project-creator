package requirement

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/project"
)

func TestSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("../../contracts/requirement.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name       string
		Input      string
		Normalized string
		Errors     []project.FieldError
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			out, errs := Validate(tc.Input)
			if out != tc.Normalized || !reflect.DeepEqual(errs, tc.Errors) {
				t.Errorf("unexpected validation result: %q %+v", out, errs)
			}
		})
	}
}

func TestRevisionReasonSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("../../contracts/revision-reason.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Input  *string
		Repeat int
		Errors []project.FieldError
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Repeat > 0 {
				value := strings.Repeat(*tc.Input, tc.Repeat)
				tc.Input = &value
			}
			if got := ValidateChangeReason(tc.Input); !reflect.DeepEqual(got, tc.Errors) {
				t.Errorf("got %v want %v", got, tc.Errors)
			}
		})
	}
}
