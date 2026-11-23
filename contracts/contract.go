// Package contracts loads the shared, versioned field constraints.
package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
)

//go:embed project.json
var projectJSON []byte

//go:embed i18n/*.json
var translations embed.FS

// Rule describes the supported constraints for one string field.
type Rule struct {
	Required  bool   `json:"required"`
	MaxLength int    `json:"maxLength"`
	Blank     string `json:"blank,omitempty"`
}

type contract struct {
	Version    int             `json:"version"`
	LengthUnit string          `json:"lengthUnit"`
	Trim       string          `json:"trim"`
	Fields     map[string]Rule `json:"fields"`
}

func parse(data []byte) (contract, error) {
	var c contract
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("expected one contract document")
	}
	if c.Version != 1 || c.LengthUnit != "unicode-code-points" || c.Trim != "ascii-space-tab-lf-cr" {
		return c, fmt.Errorf("unsupported project contract version or string semantics")
	}
	if len(c.Fields) != 3 {
		return c, fmt.Errorf("expected name, goal and description constraints")
	}
	for _, field := range []string{"name", "goal", "description"} {
		r, ok := c.Fields[field]
		if !ok || r.Blank != "" || r.MaxLength <= 0 || r.Required != (field != "description") {
			return c, fmt.Errorf("invalid constraints for %s", field)
		}
	}
	return c, nil
}

// ProjectRules returns a fresh map. Invalid embedded contracts fail at startup
// when called by the domain package, rather than silently skipping a rule.
func ProjectRules() map[string]Rule {
	c, err := parse(projectJSON)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded project contract: %v", err))
	}
	return c.Fields
}

//go:embed requirement.json
var requirementJSON []byte

// RequirementRules returns the shared requirement text and revision reason constraints.
func RequirementRules() map[string]Rule {
	var c contract
	decoder := json.NewDecoder(bytes.NewReader(requirementJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		panic(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		panic("invalid requirement contract")
	}
	text := c.Fields["text"]
	reason := c.Fields["changeReason"]
	if c.Version != 1 || c.LengthUnit != "unicode-code-points" || c.Trim != "ascii-space-tab-lf-cr" || len(c.Fields) != 2 || !text.Required || text.MaxLength <= 0 || text.Blank != "" || !reason.Required || reason.MaxLength <= 0 || reason.Blank != "unicode-white-space" {
		panic("unsupported requirement contract")
	}
	return c.Fields
}
