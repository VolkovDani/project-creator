package requirement

import (
	"encoding/json"
	"github.com/VolkovDani/project-creator/internal/project"
	"os"
	"reflect"
	"testing"
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
