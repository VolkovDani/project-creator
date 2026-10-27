package project

import (
	"strings"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/contracts"
)

var rules = contracts.ProjectRules()

// Input is the editable content of a project card.
type Input struct {
	Name        string `json:"name"`
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

// FieldError is independent of a transport and can be attached to a form field.
type FieldError struct {
	Path       []string       `json:"path"`
	Code       string         `json:"code"`
	MessageKey string         `json:"messageKey"`
	Params     map[string]int `json:"params"`
}

// Validate normalizes ASCII edge whitespace and returns all field errors.
// Length means Unicode code points; no Unicode normalization is performed.
func Validate(input Input) (Input, []FieldError) {
	errors := make([]FieldError, 0)
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"name", &input.Name}, {"goal", &input.Goal}, {"description", &input.Description},
	} {
		rule := rules[field.name]
		*field.value = strings.Trim(*field.value, " \t\r\n")
		code, key := "", ""
		params := map[string]int{}
		switch {
		case !utf8.ValidString(*field.value):
			code, key = "INVALID_UTF8", "validation.invalidEncoding"
		case rule.Required && *field.value == "":
			code, key = "REQUIRED", "validation.required"
		case utf8.RuneCountInString(*field.value) > rule.MaxLength:
			code, key = "MAX_LENGTH", "validation.maxLength"
			params["max"] = rule.MaxLength
		}
		if code != "" {
			errors = append(errors, FieldError{Path: []string{"input", field.name}, Code: code, MessageKey: key, Params: params})
		}
	}
	return input, errors
}
