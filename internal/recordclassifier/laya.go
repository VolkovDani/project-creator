// Package recordclassifier adapts the evaluated single-record decision questions to the review queue.
package recordclassifier

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/VolkovDani/project-creator/internal/classification"
)

const (
	Model           = "laya:322m-multilingual-bf16"
	Digest          = "c1242e4a83f0e1cb58f0f191a65bbc5d3306fae274d152b8229ec574999d465d"
	d3Model         = "d3:4b"
	d3Digest        = "2c571f91caf64c47b6ca6bd3adc1daa07dbb84ead8e9489758498cc7e11e533d"
	questionVersion = "flat-ru-v1"
	maxBodyBytes    = 64 * 1024
)

//go:embed flat-ru-v1.json
var flatQuestions []byte

type question struct {
	Wire   json.RawMessage
	Labels map[string]string
}

type Adapter struct {
	model, digest string
	base          string
	client        *http.Client
	questions     map[string]question
	profile       string
}

func New(base string) (*Adapter, error) {
	return newAdapter(base, Model, Digest)
}

func NewD3(base string) (*Adapter, error) {
	return newAdapter(base, d3Model, d3Digest)
}

func newAdapter(base, model, digest string) (*Adapter, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, errors.New("record classifier requires a local HTTP origin")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); host != "localhost" && host != "ollama" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("record classifier requires loopback or the local ollama service")
	}
	var questions map[string]question
	if err := json.Unmarshal(flatQuestions, &questions); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(append([]byte(model+":"+digest+":taxonomy2:ollama0.40.16:review-only-v1:"), flatQuestions...))
	return &Adapter{model: model, digest: digest, base: strings.TrimRight(base, "/"), questions: questions, profile: hex.EncodeToString(hash[:]),
		client: &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (a *Adapter) Profile() string         { return a.profile }
func (a *Adapter) Model() string           { return a.model }
func (a *Adapter) Digest() string          { return a.digest }
func (a *Adapter) QuestionVersion() string { return questionVersion }

type runtimeError int

func (e runtimeError) Error() string {
	return fmt.Sprintf("local record classifier returned HTTP %d", int(e))
}

func (a *Adapter) request(ctx context.Context, path string, body []byte, out any) error {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return runtimeError(res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, out) != nil {
		return errors.New("invalid local classifier response")
	}
	return nil
}

func (a *Adapter) verifyPin(ctx context.Context) error {
	var tags struct {
		Models []struct {
			Name, Model, Digest string
			RemoteModel         string `json:"remote_model"`
			RemoteHost          string `json:"remote_host"`
		}
	}
	if err := a.request(ctx, "/api/tags", nil, &tags); err != nil {
		return err
	}
	matches := 0
	for _, tag := range tags.Models {
		if tag.Name != a.model && tag.Model != a.model {
			continue
		}
		matches++
		if tag.RemoteModel != "" || tag.RemoteHost != "" || strings.TrimPrefix(tag.Digest, "sha256:") != a.digest {
			return errors.New("record classifier model pin mismatch")
		}
	}
	if matches != 1 {
		return errors.New("record classifier model unavailable")
	}
	return nil
}

func (a *Adapter) verifyRuntime(ctx context.Context) error {
	if err := a.verifyPin(ctx); err != nil {
		return err
	}
	var version struct{ Version string }
	if err := a.request(ctx, "/api/version", nil, &version); err != nil {
		return err
	}
	var show struct {
		Parameters string
		Details    struct {
			Format, Runner string
			Quantization   string `json:"quantization_level"`
		}
		Info struct {
			Kind   string `json:"modern-bert.decision.type"`
			D3Kind string `json:"qwen35.decision.type"`
			Head   int    `json:"modern-bert.decision.max_head_tokens"`
		} `json:"model_info"`
		Capabilities []string
	}
	payload, _ := json.Marshal(map[string]string{"model": a.model})
	if err := a.request(ctx, "/api/show", payload, &show); err != nil {
		return err
	}
	if version.Version != "0.40.16" || show.Details.Format != "gguf" || show.Details.Runner != "llamacpp" || !slices.Contains(show.Capabilities, "decision") {
		return errors.New("unsupported single-record classifier runtime")
	}
	if a.model == d3Model {
		for _, line := range strings.Split(show.Parameters, "\n") {
			if slices.Equal(strings.Fields(line), []string{"num_ctx", "4096"}) && show.Info.D3Kind == "vllm-d3" && show.Details.Quantization == "Q8_0" {
				return nil
			}
		}
		return errors.New("unsupported D3 classifier profile")
	}
	if show.Info.Kind != "laya" || show.Info.Head != 256 {
		return errors.New("unsupported Laya classifier profile")
	}
	return nil
}

type answer struct {
	Type, Choice  string
	Confidence    *float64
	Probabilities map[string]*float64
}

func validScore(v *float64) bool {
	return v != nil && *v >= 0 && *v <= 1 && !math.IsNaN(*v) && !math.IsInf(*v, 0)
}

func (a *Adapter) axis(ctx context.Context, axis, source string) ([]classification.Alternative, string, error) {
	q := a.questions[axis]
	payload, err := json.Marshal(map[string]any{"model": a.model, "state": source, "questions": map[string]json.RawMessage{"classification": q.Wire}, "keep_alive": "5m"})
	if err != nil {
		return nil, "", err
	}
	if len(payload) > maxBodyBytes {
		return nil, "input_limit", nil
	}
	var res struct {
		Model   string
		Answers map[string]answer
		Usage   struct {
			Input  *int `json:"input_tokens"`
			Output *int `json:"output_tokens"`
		}
	}
	if err = a.request(ctx, "/v1/systemone", payload, &res); err != nil {
		if err == runtimeError(http.StatusRequestEntityTooLarge) {
			return nil, "input_limit", nil
		}
		return nil, "", err
	}
	if err = a.verifyPin(ctx); err != nil {
		return nil, "", err
	}
	ans := res.Answers["classification"]
	if res.Model != a.model || res.Usage.Input == nil || res.Usage.Output == nil || *res.Usage.Input < 0 || *res.Usage.Output < 0 || len(res.Answers) != 1 || ans.Type != "choice" || q.Labels[ans.Choice] == "" || !validScore(ans.Confidence) || len(ans.Probabilities) != len(q.Labels) {
		return nil, "", errors.New("invalid single-record choice")
	}
	var items []classification.Alternative
	total := 0.0
	for key, label := range q.Labels {
		score := ans.Probabilities[key]
		if !validScore(score) {
			return nil, "", errors.New("invalid single-record option score")
		}
		item := classification.Alternative{Score: score}
		if axis == "semantic" {
			item.SemanticType = label
		} else {
			item.RepresentationType = label
		}
		items = append(items, item)
		total += *score
	}
	if math.Abs(total-1) > 1e-6 {
		return nil, "", errors.New("invalid single-record score distribution")
	}
	if a.model == d3Model && *res.Usage.Input >= 4096 {
		return nil, "input_limit", nil
	}
	if a.model == Model && *res.Usage.Input > 1024 {
		return nil, "outside_training_context", nil
	}
	sort.Slice(items, func(i, j int) bool {
		if *items[i].Score == *items[j].Score {
			return items[i].SemanticType+items[i].RepresentationType < items[j].SemanticType+items[j].RepresentationType
		}
		return *items[i].Score > *items[j].Score
	})
	return items, "", nil
}

func (a *Adapter) Classify(ctx context.Context, source string) (classification.Prediction, error) {
	result := classification.Prediction{Status: "needs_review", Semantic: "unknown", Representation: "unknown", Reason: "uncalibrated"}
	reserved := []string{"<bos>", "<eos>", "<mask>"}
	if a.model == d3Model {
		reserved = []string{
			"<|endoftext|>", "<|im_start|>", "<|im_end|>", "<|object_ref_start|>",
			"<|object_ref_end|>", "<|box_start|>", "<|box_end|>", "<|quad_start|>",
			"<|quad_end|>", "<|vision_start|>", "<|vision_end|>", "<|vision_pad|>",
			"<|image_pad|>", "<|video_pad|>", "<tool_call>", "</tool_call>",
			"<|fim_prefix|>", "<|fim_middle|>", "<|fim_suffix|>", "<|fim_pad|>",
			"<|repo_name|>", "<|file_sep|>", "<tool_response>", "</tool_response>",
			"<think>", "</think>", "<|audio_start|>", "<|audio_end|>",
			"<tts_pad>", "<tts_text_bos>", "<tts_text_eod>", "<tts_text_bos_single>",
			"<|audio_pad|>",
		}
	}
	for _, token := range reserved {
		if strings.Contains(source, token) {
			result.Reason = "reserved_token"
			return result, nil
		}
	}
	if len(source) > maxBodyBytes {
		result.Reason = "input_limit"
		return result, nil
	}
	if err := a.verifyRuntime(ctx); err != nil {
		return classification.Prediction{}, err
	}
	for _, axis := range []string{"semantic", "representation"} {
		options, reason, err := a.axis(ctx, axis, source)
		if err != nil {
			return classification.Prediction{}, err
		}
		if reason != "" {
			result.Reason = reason
			continue
		}
		result.Alternatives = append(result.Alternatives, options...)
	}
	return result, nil
}
