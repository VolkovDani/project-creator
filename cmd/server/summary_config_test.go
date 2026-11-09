package main

import "testing"

func TestSummaryRequiresExplicitLocalPinnedConfiguration(t *testing.T) {
	for _, cfg := range []map[string]string{{"PROJECT_CREATOR_SUMMARY_MODEL": "qwen2.5:7b"}, {"PROJECT_CREATOR_SUMMARY_MODEL": "qwen2.5:7b", "PROJECT_CREATOR_SUMMARY_DIGEST": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "PROJECT_CREATOR_SUMMARY_URL": "http://example.com"}} {
		if _, e := configuredSummarizer(func(k string) string { return cfg[k] }); e == nil {
			t.Fatal("accepted partial or remote summary config")
		}
	}
	g, e := configuredSummarizer(func(string) string { return "" })
	if e != nil || g != nil {
		t.Fatal("unconfigured summaries must not call models")
	}
}

func TestSummaryLanguagesAreExplicitAndValidated(t *testing.T) {
	cfg := map[string]string{
		"PROJECT_CREATOR_SUMMARY_MODEL":     "local-summary-test",
		"PROJECT_CREATOR_SUMMARY_DIGEST":    "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		"PROJECT_CREATOR_SUMMARY_LANGUAGES": "en,de",
	}
	g, err := configuredSummarizer(func(k string) string { return cfg[k] })
	if err != nil {
		t.Fatal(err)
	}
	validator, ok := g.(interface{ ValidateSummary(string) error })
	if !ok || validator.ValidateSummary("Prüfung 42 €") != nil || validator.ValidateSummary("Проверка") == nil {
		t.Fatal("language configuration not applied")
	}
	cfg["PROJECT_CREATOR_SUMMARY_LANGUAGES"] = "ru,not-a-supported-language"
	if _, err := configuredSummarizer(func(k string) string { return cfg[k] }); err == nil {
		t.Fatal("invalid language configuration silently accepted")
	}
}
