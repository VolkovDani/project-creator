package main

import "testing"

func TestRecordClassifierRequiresExplicitLocalReview(t *testing.T) {
	env := map[string]string{"PROJECT_CREATOR_OLLAMA_URL": "http://ollama:11434"}
	get := func(key string) string { return env[key] }
	c, err := configuredRecordClassifier(get)
	if err != nil || c != nil {
		t.Fatal("unexpected default classifier", err)
	}
	env["PROJECT_CREATOR_RECORD_CLASSIFIER"] = "laya-review"
	c, err = configuredRecordClassifier(get)
	if err != nil || c == nil || c.Model() != "laya:322m-multilingual-bf16" {
		t.Fatal("missing local adapter", err)
	}
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_URL"] = "https://example.org"
	if _, err := configuredRecordClassifier(get); err == nil {
		t.Fatal("external URL accepted")
	}
	env["PROJECT_CREATOR_RECORD_CLASSIFIER"] = "other"
	if _, err := configuredRecordClassifier(get); err == nil {
		t.Fatal("unexpected provider accepted")
	}
}
