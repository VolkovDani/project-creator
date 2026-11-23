package requirement

import (
	"strings"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/project"
)

var rules = contracts.RequirementRules()

func Validate(text string) (string, []project.FieldError) {
	text = strings.Trim(text, " \t\r\n")
	return text, validateField("text", text, text == "")
}

func ValidateChangeReason(reason *string) []project.FieldError {
	value := ""
	if reason != nil {
		value = *reason
	}
	return validateField("changeReason", value, strings.TrimSpace(value) == "")
}

func validateField(field, value string, blank bool) []project.FieldError {
	rule := rules[field]
	code, key := "", ""
	params := map[string]int{}
	switch {
	case !utf8.ValidString(value):
		code, key = "INVALID_UTF8", "validation.invalidEncoding"
	case rule.Required && blank:
		code, key = "REQUIRED", "validation.required"
	case utf8.RuneCountInString(value) > rule.MaxLength:
		code, key = "MAX_LENGTH", "validation.maxLength"
		params["max"] = rule.MaxLength
	}
	if code != "" {
		return []project.FieldError{{Path: []string{"input", field}, Code: code, MessageKey: key, Params: params}}
	}
	return []project.FieldError{}
}
