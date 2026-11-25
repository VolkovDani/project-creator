package analysis

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

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

func TestSummarySourceIdentifierBoundaries(t *testing.T) {
	policy, err := NewSummaryLanguagePolicy("ru")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, output string
		valid                bool
	}{
		{"sentence punctuation and case", "Сохранить в excel.", "Экспортировать в Excel!", true},
		{"arbitrary lowercase name", "Открыть zeta.", "Открывается Zeta.", true},
		{"foreign prose source", "Описание: export all records.", "Нужно export all records.", false},
		{"punctuated foreign prose", "Описание: export, all, records.", "Нужно export all records.", false},
		{"isolated name after foreign prose", "Действие export all records, формат Excel.", "Формат Excel.", true},
		{"part of prose is not an isolated name", "Действие export all records, формат Excel.", "Формат records.", false},
		{"code identifiers in a source expression", "Повернуть массив. dst[i] = src[i];", "Перенести элемент src в dst по индексу i.", true},
		{"invented code identifier", "Повернуть массив. dst[i] = src[i];", "Сохранить в other.", false},
		{"structured identifier in a prose-free version listing", "pkg2/3.1", "Указана версия pkg2/3.1.", true},
		{"unrelated identifier in a prose-free listing", "pkg2/3.1", "Указана версия pkg3/3.1.", false},
		{"code-only source", "dst[i] = src[i];", "Копирует элемент src в dst.", true},
		{"code alone is not target language prose", "dst[i] = src[i];", "dst[i] = src[i];", false},
		{"parenthesized technical name", "Режим SSO (Single Sign-On) включён.", "Включён SSO (Single Sign-On).", true},
		{"do not admit individual words from a long name", "Режим SSO (Single Sign-On) включён.", "Включён Single.", false},
		{"new name", "Экспорт в Excel.", "Экспорт в Word.", false},
		{"foreign-only source", "Excel", "Файл Excel.", false},
		{"foreign-only output", "Экспорт в Excel.", "Excel", false},
		{"combining mark", "Экспорт в Excel.", "Экспорт в Excel\u0301.", false},
		{"hidden control", "Экспорт в Excel.", "Экспорт в Excel\u200b.", false},
		{"mixed-script lookalike", "Экспорт в Excel.", "Экспорт в Excеl.", false},
		{"structured identifier", "Запрос через api_v2.", "Нужен api_v2.", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.ValidForSource(tc.output, tc.source); got != tc.valid {
				t.Fatalf("ValidForSource = %v, want %v", got, tc.valid)
			}
		})
	}
}

func TestSummarySourceAlphabetSelection(t *testing.T) {
	for _, tc := range []struct{ config, source, want string }{
		{"ru,en", "Только аудитор скачивает отчёт через API.", "ru"},
		{"ru,en", "Only auditors may download the report.", "en"},
		{"ru,en", "Аудитор проверяет отчёт. Only admins can export.", "en,ru"},
		{"ru,en", "123", "en,ru"},
		{"ru,en", "中国的规定", "en,ru"},
		{"ru,en,fr", "Export the report.", "en,fr"},
	} {
		policy, err := NewSummaryLanguagePolicy(tc.config)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(policy.forSource(tc.source).Codes(), ","); got != tc.want {
			t.Errorf("%s / %q: got %s, want %s", tc.config, tc.source, got, tc.want)
		}
	}
}

func TestLanguageRepairGrammarKeepsSourceIdentifiersAndRequiresSupportedSpelling(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("ru")
	re := regexp.MustCompile(p.repairPattern("Проверить SSO (Single Sign-On) через api_v2, затем dst[i] = src[i] при ≥ 50 °С."))
	for text, want := range map[string]bool{"Проверить api_v2.": true, "Режим SSO (Single Sign-On).": true, "Элемент src.": true, "Температура ≥ 50 °С.": true, "Новые foreign words": false, "Вставка 中文": false} {
		if got := re.MatchString(text); got != want {
			t.Errorf("%q: %v", text, got)
		}
	}
}

func TestLanguageRepairDoesNotConcatenateOverlappingNames(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("ru")
	re := regexp.MustCompile(p.repairPattern("Открыть A и AA при 12 единицах."))
	if re.MatchString("Значение AAA") || !re.MatchString("Значения AA, A и 12.") {
		t.Fatal("overlapping names must have a separator")
	}
}

func TestLanguageRepairUsesOnlyOneClassForNativeNames(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("en")
	if strings.Contains(p.repairPattern("Enable SSO (Single Sign-On)."), "|") {
		t.Fatal("English source names must not duplicate the native alphabet")
	}
}

func TestSummaryKeepsGroundedTypedParameterAndAnnotation(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("ru")
	source := "Проверить список. @param entries — вход. function check(entries: number[]) {}"
	for _, text := range []string{"Проверяет параметр entries.", "Аннотация @param описывает вход."} {
		if !p.ValidForSource(text, source) {
			t.Errorf("rejected %q", text)
		}
	}
}

func TestLanguageRepairKeepsWholeTechnicalWords(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("ru")
	source := "Формат config_v2 и версия Q7/12.5. Использовать API."
	re := regexp.MustCompile(p.repairPattern(source))
	for text, want := range map[string]bool{"Результат config_v2, версия Q7/12.5.": true, "Принять «API».": true, "Использовать config.": false, "Имя яAPI": false, "Имя APIя": false, "Имя API-объект": false, "Имя Q8.": false} {
		if got := re.MatchString(text); got != want {
			t.Errorf("%q: got %v, want %v", text, got, want)
		}
	}
}

func TestLanguageRepairRequiresConfiguredLanguageAlongsideIdentifiers(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("ru")
	source := "QA-42, OPS-19, TICKET-55"
	re := regexp.MustCompile(p.repairPattern(source))
	for _, output := range []string{source, "QA-42.", "42, 19, 55"} {
		if re.MatchString(output) {
			t.Errorf("repair grammar accepted a preview without Russian wording: %q", output)
		}
	}
	for _, output := range []string{"Указаны QA-42, OPS-19 и TICKET-55.", "QA-42, OPS-19 и TICKET-55."} {
		if !re.MatchString(output) || !p.ValidForSource(output, source) {
			t.Errorf("repair grammar or validator rejected a grounded preview: %q", output)
		}
	}
}

func TestOrderedSummaryRepairKeepsCompleteNumericSourceTokens(t *testing.T) {
	p, _ := NewSummaryLanguagePolicy("ru")
	source := "Дата 2026-11-25, режим config_v2 и пакет pkg-2.1. Порог 1,25, задержка 50мс, код 42. Повторить код 42."
	numbers := summaryRepairNumbers(source)
	want := []string{"2026-11-25", "config_v2", "pkg-2.1", "1,25", "50мс", "42"}
	if !slices.Equal(numbers, want) {
		t.Fatalf("numeric source tokens %q, want %q", numbers, want)
	}
	pattern := regexp.MustCompile(p.repairPattern(source, numbers...))
	valid := "Дата 2026-11-25, режим config_v2, пакет pkg-2.1. Порог 1,25, задержка 50мс, код 42."
	if !pattern.MatchString(valid) || !p.ValidForSource(valid, source) || len(missingSummaryNumbers(source, valid)) != 0 {
		t.Fatal("required values and supported prose must fit both the schema and publication checks")
	}
	for _, invalid := range []string{
		"Дата 2026-11-25, режим config_v2, пакет pkg-2.1. Порог 1,25, задержка 50мс.",
		"Дата 2026-11-25, пакет pkg-2.1, режим config_v2. Порог 1,25, задержка 50мс, код 42.",
		strings.Replace(valid, "config_v2", "2", 1),
		strings.Replace(valid, "1,25", "1, 25", 1),
		strings.Replace(valid, "50мс", "50", 1),
		strings.Replace(valid, "код 42", "код 142", 1),
		"2026-11-25, config_v2, pkg-2.1, 1,25, 50мс, 42.",
	} {
		if pattern.MatchString(invalid) {
			t.Errorf("numeric repair schema accepted %q", invalid)
		}
	}
}
