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
	model := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL")
	digest := getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST")
	if mode == "laya-review" {
		model, digest = recordclassifier.Model, recordclassifier.Digest
	} else if mode != "systemone-review" {
		return nil, errors.New("unknown PROJECT_CREATOR_RECORD_CLASSIFIER; expected systemone-review or laya-review")
	}
	var adapter *recordclassifier.Adapter
	var err error
	switch getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_RUNTIME") {
	case "", "ollama":
		if endpoint == "" {
			endpoint = getenv("PROJECT_CREATOR_OLLAMA_URL")
		}
		if endpoint == "" {
			endpoint = "http://127.0.0.1:11434"
		}
		adapter, err = recordclassifier.NewSystemOne(endpoint, model, digest)
	case "managed":
		adapter, err = recordclassifier.NewManaged(endpoint, getenv("PROJECT_CREATOR_RECORD_CLASSIFIER_PROFILE"), model, digest)
	default:
		return nil, errors.New("unknown PROJECT_CREATOR_RECORD_CLASSIFIER_RUNTIME; expected ollama or managed")
	}
	if err != nil {
		return nil, err
	}
	return adapter, nil
}
