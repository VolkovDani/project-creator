package main

import (
	"errors"

	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/recordclassifier"
)

func configuredRecordClassifier(getenv func(string) string) (classification.Classifier, error) {
	mode := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER")
	if mode == "" {
		return nil, nil
	}
	endpoint := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_URL")
	if endpoint == "" {
		endpoint = getenv("PROJECT_CREATOR_OLLAMA_URL")
	}
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	model := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL")
	digest := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST")
	if mode == "laya-review" {
		model, digest = recordclassifier.Model, recordclassifier.Digest
	} else if mode != "systemone-review" {
		return nil, errors.New("unknown PROJECT_CREATOR_RECORD_CLASSIFIER; expected systemone-review or laya-review")
	}
	adapter, err := recordclassifier.NewSystemOne(endpoint, model, digest)
	if err != nil {
		return nil, err
	}
	return adapter, nil
}
