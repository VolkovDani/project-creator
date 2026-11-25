package glossary

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
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

func TestPairHintsMatchBothSourcesAndPreserveAmbiguity(t *testing.T) {
	terms := []Term{
		{ID: "a", Name: "Генеральный подрядчик", Version: 2, Definition: "Организация", Context: "Строительство", Variants: []Variant{{"ГП", "abbreviation"}, {"Генподрядчик", "synonym"}}},
		{ID: "b", Name: "Генеральный план", Version: 3, Definition: "Документ", Variants: []Variant{{"ГП", "abbreviation"}}},
		{ID: "c", Name: "Подрядчик", Version: 1},
		{ID: "d", Name: "ГПУ", Version: 1},
	}
	left := "😀 ГП, ГПУ2 и генеральный подрядчик. генподрядчик."
	right := "Гп и Генподрядчик. Генеральный план."
	want := []Hint{
		{TermID: "b", Version: 3, Spelling: "Генеральный план", Name: "Генеральный план", Definition: "Документ"},
		{TermID: "a", Version: 2, Spelling: "генеральный подрядчик", Name: "Генеральный подрядчик", Definition: "Организация", Context: "Строительство"},
		{TermID: "a", Version: 2, Spelling: "Генподрядчик", Name: "Генеральный подрядчик", Definition: "Организация", Context: "Строительство"},
		{Ambiguous: true, Spelling: "ГП"},
	}
	got := PairHints(terms)(left, right)
	if !slices.Equal(got, want) {
		t.Fatalf("hints = %+v, want %+v", got, want)
	}
	slices.Reverse(terms)
	reversed := PairHints(terms)(right, left)
	if !slices.Equal(got, reversed) {
		t.Fatalf("reversed hints = %+v, want %+v", reversed, got)
	}
	if empty, _ := json.Marshal(PairHints(terms)("ГПУ2", "_генподрядчик")); string(empty) != "[]" {
		t.Fatalf("empty hints = %s", empty)
	}
}

func TestPairHintsFitEncodedBudgetDeterministically(t *testing.T) {
	var terms []Term
	var labels []string
	for i := 0; i < 25; i++ {
		label := fmt.Sprintf("Смысл%02d", i)
		labels = append(labels, label)
		terms = append(terms, Term{ID: label, Version: 1, Name: label, Definition: strings.Repeat("😀<&", 50)})
	}
	hints := PairHints(terms)
	left, right := strings.Join(labels[:10], " "), strings.Join(labels[10:], " ")
	got := hints(left, right)
	if len(got) == 0 || len(got) >= len(terms) {
		t.Fatalf("expected truncated, nonempty hints, got %d", len(got))
	}
	if encoded, _ := json.Marshal(got); len(encoded) > 3072 {
		t.Fatalf("encoded hints use %d bytes", len(encoded))
	}
	if reversed := hints(right, left); !slices.Equal(got, reversed) {
		t.Fatalf("reversed hints = %+v, want %+v", reversed, got)
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

func TestCandidateContextClipping(t *testing.T) {
	before := strings.Repeat("😀", 59) + " "
	after := " " + strings.Repeat("я", 59)
	for _, tc := range []struct {
		name, prefix, suffix            string
		beforeTruncated, afterTruncated bool
	}{
		{"exact fit", "", "", false, false},
		{"start omitted", "x", "", true, false},
		{"end omitted", "", "x", false, true},
		{"both omitted", "x", "x", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Candidates(tc.prefix + before + "ГП" + after + tc.suffix)
			if len(got) != 1 {
				t.Fatal(got)
			}
			c := got[0]
			if c.Before != before || c.After != after || c.BeforeTruncated != tc.beforeTruncated || c.AfterTruncated != tc.afterTruncated {
				t.Fatalf("unexpected candidate context: %+v", c)
			}
			if c.Start != 60+len(tc.prefix) || c.End != c.Start+2 {
				t.Fatalf("unexpected mention offsets: %d–%d", c.Start, c.End)
			}
		})
	}
}
