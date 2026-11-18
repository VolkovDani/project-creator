package contracts

import (
	_ "embed"
	"encoding/json"
)

//go:embed classification.json
var classificationJSON []byte

type ClassificationContract struct {
	Version                          string
	Semantic, Representation, States []string
}

func ClassificationRules() ClassificationContract {
	var rules ClassificationContract
	if err := json.Unmarshal(classificationJSON, &rules); err != nil {
		panic(err)
	}
	return rules
}
