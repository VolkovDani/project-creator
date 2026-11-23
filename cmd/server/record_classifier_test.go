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
	layaProfile := c.Profile()
	env["PROJECT_CREATOR_RECORD_CLASSIFIER"] = "systemone-review"
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL"] = c.Model()
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST"] = c.Digest()
	c, err = configuredRecordClassifier(get)
	if err != nil || c == nil || c.Profile() != layaProfile {
		t.Fatal("explicit Laya configuration changed the historical profile", err)
	}
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL"] = "d3:4b"
	delete(env, "PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST")
	if _, err := configuredRecordClassifier(get); err == nil {
		t.Fatal("missing digest accepted")
	}
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST"] = "sha256:2c571f91caf64c47b6ca6bd3adc1daa07dbb84ead8e9489758498cc7e11e533d"
	c, err = configuredRecordClassifier(get)
	if err != nil || c == nil || c.Model() != "d3:4b" || c.Digest() != "2c571f91caf64c47b6ca6bd3adc1daa07dbb84ead8e9489758498cc7e11e533d" || c.Profile() == layaProfile {
		t.Fatal("missing distinct pinned D3 profile", err)
	}
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL"] = "other-local-decision:1b"
	env["PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST"] = "8862558d6820aa79def12b502bf021ba9149d29f51cbdf02e79bcaf71f073715"
	c, err = configuredRecordClassifier(get)
	if err != nil || c == nil || c.Model() != env["PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL"] || c.Digest() != env["PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST"] {
		t.Fatal("configured model was not selected", err)
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
