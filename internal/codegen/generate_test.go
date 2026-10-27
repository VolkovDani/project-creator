package codegen

import (
	"strings"
	"testing"
)

func TestGenerateRejectsUnsupported(t *testing.T) {
	for _, s := range []string{`scalar Date`, `type A { x: Float! }`, `type A @private { x: String! }`} {
		if _, err := Generate(s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestGenerateFields(t *testing.T) {
	out, err := Generate(`input ProjectInput { name: String! } type Project { id: ID! }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
}

func TestRequiredInputUsesValue(t *testing.T) {
	out, err := Generate(`input P { name: String! } type Mutation { add(input: P!): String! }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Input P\n") {
		t.Fatalf("required input must be a value: %s", out)
	}
}
