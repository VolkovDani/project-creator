package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/VolkovDani/project-creator/internal/oaicompat"
)

type OpenAICompatible struct {
	client     *oaicompat.Client
	dimensions int
	profile    string
}

// NewOpenAICompatible uses the model's native embedding dimensions.
func NewOpenAICompatible(baseURL, model, key string) (*OpenAICompatible, error) {
	return newOpenAICompatible(baseURL, model, key, 0)
}

// NewOpenAICompatibleWithDimensions requests the storage dimension from models
// that support the optional dimensions parameter.
func NewOpenAICompatibleWithDimensions(baseURL, model, key string) (*OpenAICompatible, error) {
	return newOpenAICompatible(baseURL, model, key, Dimensions)
}

func newOpenAICompatible(baseURL, model, key string, dimensions int) (*OpenAICompatible, error) {
	client, err := oaicompat.New(baseURL, model, key)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte("openai-compatible-embeddings-v1\n" + client.Identity() +
		"\n" + strconv.Itoa(dimensions) + "\n" + embeddingIndexFormat))
	return &OpenAICompatible{client: client, dimensions: dimensions, profile: hex.EncodeToString(hash[:])}, nil
}

func (p *OpenAICompatible) Profile() string { return p.profile }

// JSON null would silently become a zero component when unmarshaled into []float32.
type embeddingVector []float32

func (v *embeddingVector) UnmarshalJSON(data []byte) error {
	if bytes.Contains(data, []byte("null")) {
		return errors.New("embedding contains a null")
	}
	return json.Unmarshal(data, (*[]float32)(v))
}

func (p *OpenAICompatible) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := validateEmbeddingInput(texts); err != nil {
		return nil, err
	}
	body := struct {
		Model          string   `json:"model"`
		Input          []string `json:"input"`
		EncodingFormat string   `json:"encoding_format"`
		Dimensions     int      `json:"dimensions,omitempty"`
	}{p.client.Model(), texts, "float", p.dimensions}
	var response struct {
		Data []struct {
			Index     *int            `json:"index"`
			Embedding embeddingVector `json:"embedding"`
		} `json:"data"`
	}
	if err := p.client.Post(ctx, "/embeddings", body, &response); err != nil {
		return nil, err
	}
	if len(response.Data) != len(texts) {
		return nil, errors.New("embedding result count mismatch")
	}
	vectors := make([][]float32, len(texts))
	for _, item := range response.Data {
		if item.Index == nil || *item.Index < 0 || *item.Index >= len(vectors) || vectors[*item.Index] != nil {
			return nil, errors.New("invalid embedding result index")
		}
		if err := validateEmbeddingVector(item.Embedding); err != nil {
			return nil, err
		}
		vectors[*item.Index] = item.Embedding
	}
	return vectors, nil
}
