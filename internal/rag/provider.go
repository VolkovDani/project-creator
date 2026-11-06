package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const Dimensions = 1024
const maximumResponse = 8 * 1024 * 1024

// Embedder is replaceable without coupling retrieval to a model runtime.
// Profile must change whenever model weights or preprocessing change.
type Embedder interface {
	Profile() string
	Embed(context.Context, []string) ([][]float32, error)
}

type Ollama struct {
	baseURL, model, digest, profile string
	client                          *http.Client
}

var digestPattern = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)

// NewOllama accepts loopback runtimes or the literal Docker service name ollama.
// The latter is an explicit local Compose configuration, never a fallback.
// It never downloads a model,
// follows redirects or falls back to a cloud endpoint. Digest pins model weights.
func NewOllama(baseURL, model, expectedDigest string) (*Ollama, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("Ollama requires a loopback HTTP URL")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && host != "ollama" && host != "ollama-embeddings" && host != "ollama-generator" && host != "ollama-laya" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("Ollama requires a loopback HTTP URL")
	}
	if strings.TrimSpace(model) != model || model == "" || len(model) > 200 || strings.ContainsAny(model, "\r\n") || !digestPattern.MatchString(expectedDigest) {
		return nil, errors.New("Ollama model and full SHA256 digest are required")
	}
	digest := strings.TrimPrefix(expectedDigest, "sha256:")
	profileHash := sha256.Sum256([]byte(model + "\n" + digest + "\n1024\nchunking-v1:1000:100"))
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Ollama{baseURL: strings.TrimRight(baseURL, "/"), model: model, digest: digest, profile: hex.EncodeToString(profileHash[:]), client: &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects disabled") }}}, nil
}
func (o *Ollama) Profile() string { return o.profile }

func (o *Ollama) request(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, o.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid local model request")
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := o.client.Do(req)
	if err != nil {
		return errors.New("local embedding runtime unavailable or request cancelled")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("local embedding runtime rejected request")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumResponse+1))
	if err != nil || len(data) > maximumResponse {
		return errors.New("invalid local model response size")
	}
	if err = json.Unmarshal(data, out); err != nil {
		return errors.New("invalid local model response")
	}
	return nil
}
func (o *Ollama) verifyModel(ctx context.Context) error {
	var response struct {
		Models []struct {
			Name        string `json:"name"`
			Model       string `json:"model"`
			Digest      string `json:"digest"`
			RemoteHost  string `json:"remote_host"`
			RemoteModel string `json:"remote_model"`
		} `json:"models"`
	}
	if err := o.request(ctx, "GET", "/api/tags", nil, &response); err != nil {
		return err
	}
	for _, model := range response.Models {
		if model.Name == o.model || model.Model == o.model {
			if model.RemoteHost != "" || model.RemoteModel != "" {
				return errors.New("remote embedding models are disabled")
			}
			if strings.TrimPrefix(model.Digest, "sha256:") != o.digest {
				return errors.New("embedding model digest changed; configure a new profile and reindex")
			}
			return nil
		}
	}
	return errors.New("pinned embedding model is not installed locally")
}
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 || len(texts) > 32 {
		return nil, errors.New("embedding batch must contain 1 to 32 texts")
	}
	for _, text := range texts {
		if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 10000 {
			return nil, errors.New("invalid embedding input")
		}
	}
	if err := o.verifyModel(ctx); err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		Model    string   `json:"model"`
		Input    []string `json:"input"`
		Truncate bool     `json:"truncate"`
	}{o.model, texts, false})
	if err != nil {
		return nil, errors.New("invalid embedding input")
	}
	var response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err = o.request(ctx, "POST", "/api/embed", body, &response); err != nil {
		return nil, err
	}
	if len(response.Embeddings) != len(texts) {
		return nil, errors.New("embedding result count mismatch")
	}
	for _, vector := range response.Embeddings {
		if len(vector) != Dimensions {
			return nil, errors.New("embedding dimension mismatch")
		}
		var norm float64
		for _, value := range vector {
			v := float64(value)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, errors.New("nonfinite embedding")
			}
			norm += v * v
		}
		if norm == 0 {
			return nil, errors.New("zero embedding is not searchable")
		}
	}
	if err = o.verifyModel(ctx); err != nil {
		return nil, err
	}
	return response.Embeddings, nil
}
