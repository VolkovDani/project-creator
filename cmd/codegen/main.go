package main

import (
	"github.com/VolkovDani/project-creator/internal/codegen"
	"log"
	"os"
)

func main() {
	b, e := os.ReadFile("graph/schema.graphqls")
	if e != nil {
		log.Fatal(e)
	}
	out, e := codegen.Generate(string(b))
	if e != nil {
		log.Fatal(e)
	}
	if e = os.WriteFile("graph/models_gen.go", out, 0644); e != nil {
		log.Fatal(e)
	}
}
