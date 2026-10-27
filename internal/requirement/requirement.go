package requirement

import (
	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/project"
	"strings"
	"unicode/utf8"
)

var rule = contracts.RequirementRules()["text"]

func Validate(text string) (string, []project.FieldError) {
	text = strings.Trim(text, " \t\r\n")
	code, key := "", ""
	params := map[string]int{}
	switch {
	case !utf8.ValidString(text):
		code, key = "INVALID_UTF8", "validation.invalidEncoding"
	case text == "":
		code, key = "REQUIRED", "validation.required"
	case utf8.RuneCountInString(text) > rule.MaxLength:
		code, key = "MAX_LENGTH", "validation.maxLength"
		params["max"] = rule.MaxLength
	}
	if code != "" {
		return text, []project.FieldError{{Path: []string{"input", "text"}, Code: code, MessageKey: key, Params: params}}
	}
	return text, []project.FieldError{}
}
