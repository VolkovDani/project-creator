package main

import (
	"errors"
	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/recordclassifier"
)

func configuredRecordClassifier(getenv func(string) string) (classification.Classifier, error) {
	mode := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER")
	switch mode {
	case "":
		return nil, nil
	case "laya-review", "d3-review":
		endpoint := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_URL")
		if endpoint == "" {
			endpoint = getenv("PROJECT_CREATOR_OLLAMA_URL")
		}
		if endpoint == "" {
			endpoint = "http://127.0.0.1:11434"
		}
		newAdapter := recordclassifier.New
		if mode == "d3-review" {
			newAdapter = recordclassifier.NewD3
		}
		adapter, err := newAdapter(endpoint)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	default:
		return nil, errors.New("unknown PROJECT_CREATOR_RECORD_CLASSIFIER; expected laya-review or d3-review")
	}
}
