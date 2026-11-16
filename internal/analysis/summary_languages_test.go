package analysis

import "testing"

func TestSummaryLanguagePolicy(t *testing.T) {
	policy, err := NewSummaryLanguagePolicy("")
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.Codes(); len(got) != 2 || got[0] != "en" || got[1] != "ru" {
		t.Fatalf("codes = %v", got)
	}
	for _, text := range []string{"Report 123 👩‍🔬 1️⃣", "Текст Ёж"} {
		if !policy.Valid(text) {
			t.Errorf("Valid(%q) = false", text)
		}
	}
	for _, text := range []string{"中文", "Ελληνικά", "їі", "ñ", "a\u0301", "\ue000", "\u0378", "\u200b", "\a"} {
		if policy.Valid(text) {
			t.Errorf("Valid(%q) = true", text)
		}
	}
	en, err := NewSummaryLanguagePolicy(" en, en ")
	if err != nil {
		t.Fatal(err)
	}
	if en.Valid("Русский") {
		t.Fatal("English-only policy accepted Russian")
	}
	de, err := NewSummaryLanguagePolicy("de,en")
	if err != nil {
		t.Fatal(err)
	}
	if !de.Valid("Grüße") {
		t.Fatal("German alphabet rejected")
	}
	zh, err := NewSummaryLanguagePolicy("zh,en")
	if err != nil {
		t.Fatal(err)
	}
	if !zh.Valid("中文") {
		t.Fatal("Chinese script rejected")
	}
}

func TestSummaryLanguagePolicyRejectsBadCodes(t *testing.T) {
	for _, codes := range []string{"en,,ru", "xx", " , "} {
		if _, err := NewSummaryLanguagePolicy(codes); err == nil {
			t.Errorf("accepted %q", codes)
		}
	}
}

func TestSummaryLanguagePolicyCanonicalProfileInputs(t *testing.T) {
	a, _ := NewSummaryLanguagePolicy("ru,en,ru")
	b, _ := NewSummaryLanguagePolicy("en,ru")
	if a.Instruction() != b.Instruction() {
		t.Fatal("equivalent lists have different instructions")
	}
	codes := a.Codes()
	codes[0] = "zh"
	if a.Codes()[0] != "en" {
		t.Fatal("caller mutated language policy")
	}
	uk, _ := NewSummaryLanguagePolicy("uk")
	if !uk.Valid("Їжак ґрунт Єва") || uk.Valid("ыэёъ") {
		t.Fatal("incorrect Ukrainian alphabet")
	}
}
