package contracts

import (
	_ "embed"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

//go:embed summary.json
var summaryJSON []byte

// SummaryMaxBytes reads the shared UTF-8 byte budget. The limit applies only to Laya. Whitespace and output
// encoding are validated without altering the generated text.
func SummaryMaxBytes() int {
	var c struct {
		Version    int    `json:"version"`
		LengthUnit string `json:"lengthUnit"`
		MaxBytes   int    `json:"layaMaxBytes"`
		Required   bool   `json:"required"`
	}
	if json.Unmarshal(summaryJSON, &c) != nil || c.Version != 2 || c.LengthUnit != "utf8-bytes" || c.MaxBytes <= 0 || !c.Required {
		panic("invalid summary contract")
	}
	return c.MaxBytes
}
func ValidSummary(text string) bool {
	return utf8.ValidString(text) && strings.TrimSpace(text) != "" && !strings.ContainsRune(text, 0)
}

// SummaryLayaEligible is independent of whether a preview can be stored.
func SummaryLayaEligible(text string) bool {
	return ValidSummary(text) && len(text) <= SummaryMaxBytes()
}
