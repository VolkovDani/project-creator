package contracts

import (
	"strings"
	"testing"
)

func TestRejectUnsupportedContracts(t *testing.T) {
	for _, change := range []struct{ old, new string }{
		{`"version": 1`, `"version": 2`},
		{`"unicode-code-points"`, `"bytes"`},
		{`"ascii-space-tab-lf-cr"`, `"unicode-whitespace"`},
		{`"description"`, `"unknownField"`},
		{`"maxLength": 120`, `"maxLength": -1`},
		{`"required": true`, `"required": false`},
	} {
		data := strings.Replace(string(projectJSON), change.old, change.new, 1)
		if data == string(projectJSON) {
			t.Fatal("test mutation did not apply")
		}
		if _, err := parse([]byte(data)); err == nil {
			t.Errorf("unsupported contract accepted: %s", change.new)
		}
	}
	if _, err := parse(append(append([]byte{}, projectJSON...), []byte(` {}`)...)); err == nil {
		t.Error("trailing JSON accepted")
	}
}

func TestTranslationsCoverValidationKeys(t *testing.T) {
	for _, locale := range []string{"ru", "en"} {
		dictionary := Dictionary(locale)
		for _, key := range []string{"validation.required", "validation.maxLength", "validation.invalidEncoding"} {
			if dictionary[key] == "" {
				t.Errorf("%s missing %s", locale, key)
			}
		}
		if strings.Count(dictionary["validation.maxLength"], "{max}") != 1 {
			t.Errorf("%s missing max placeholder", locale)
		}
	}
}
