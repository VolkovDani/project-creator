package project

// Input is the editable content of a project card.
type Input struct {
 Name string `json:"name"`
 Goal string `json:"goal"`
 Description string `json:"description"`
}

// FieldError is independent of a transport and can be attached to a form field.
type FieldError struct {
 Path []string `json:"path"`
 Code string `json:"code"`
 MessageKey string `json:"messageKey"`
 Params map[string]int `json:"params"`
}

func Validate(input Input) (Input, []FieldError) { return input, nil }
