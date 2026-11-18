package contracts

import (
	_ "embed"
	"encoding/json"
)

//go:embed glossary.json
var glossaryJSON []byte

type GlossaryConstraints struct {
	NameMax, DefinitionMax, ContextMax, VariantMax, VariantLimit int
	Kinds                                                        []string
}

func GlossaryRules() GlossaryConstraints {
	var c GlossaryConstraints
	if err := json.Unmarshal(glossaryJSON, &c); err != nil {
		panic(err)
	}
	return c
}
