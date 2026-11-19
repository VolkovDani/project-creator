package glossary

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLongestWholeTermAndAmbiguity(t *testing.T) {
	terms := []Term{{ID: "a", Name: "Генеральный подрядчик", Variants: []Variant{{"ГП", "abbreviation"}, {"Генподрядчик", "synonym"}}}, {ID: "b", Name: "Подрядчик"}, {ID: "c", Name: "Генеральный план", Variants: []Variant{{"ГП", "abbreviation"}}}}
	s := "ГПУ, генеральный подрядчик; Генподрядчик и ГП."
	got := Matches(s, terms)
	if len(got) != 3 || got[0].Text != "генеральный подрядчик" || got[1].Text != "Генподрядчик" || len(got[2].Terms) != 2 {
		t.Fatalf("matches=%+v", got)
	}
}

func TestDiscoverExpansionRequiresMatchingInitials(t *testing.T) {
	got := Candidates("Применяется Генеральный подрядчик (ГП). Отчёт (ГП).")
	if len(got) != 1 || got[0].Name != "Генеральный подрядчик" || got[0].Abbreviation != "ГП" {
		t.Fatalf("%+v", got)
	}
}

func TestSummaryHintsExcludeAmbiguityAndRetainVersions(t *testing.T) {
	a := Term{ID: "a", Version: 2, Name: "Генеральный подрядчик", Definition: "Организация", Variants: []Variant{{"ГП", "abbreviation"}, {"Генподрядчик", "synonym"}}}
	b := Term{ID: "b", Version: 3, Name: "Генеральный план", Variants: []Variant{{"ГП", "abbreviation"}}}
	hints := SummaryHints("ГП и генподрядчик", []Term{a, b})
	if len(hints) != 2 || hints[0].TermID != "" || hints[0].Definition != "" || hints[0].Spelling != "ГП" || hints[1].TermID != "a" || hints[1].Version != 2 {
		t.Fatal(hints)
	}
}

func TestGlossarySharedValidationFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../contracts/glossary.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Code string
		Term       Term
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got := ""
		if errs := validate(&c.Term); len(errs) > 0 {
			got = errs[0].Code
		}
		if got != c.Code {
			t.Errorf("%s: %s", c.Name, got)
		}
	}
}

func TestStandaloneAbbreviation(t *testing.T) {
	got := Candidates("😀 ГП, ГП. ГП2 ГПшка xAPI _ТЗ АБВГДЕЖЗИ")
	if len(got) != 1 || got[0].Name != "" || got[0].Abbreviation != "ГП" {
		t.Fatal(got)
	}
}
