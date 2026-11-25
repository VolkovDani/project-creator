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
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/VolkovDani/project-creator/internal/classification"
)

const (
	Model           = "laya:322m-multilingual-bf16"
	Digest          = "c1242e4a83f0e1cb58f0f191a65bbc5d3306fae274d152b8229ec574999d465d"
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

// Extra constraints measured for individual artifacts apply to every local alias of the same digest.
type measuredProfile struct {
	modelInfo         map[string]any
	quantization      string
	configuredContext int
	trainingContext   int
	reserved          []string
}

var measuredProfiles = map[string]measuredProfile{
	Digest: {
		modelInfo:       map[string]any{"modern-bert.decision.type": "laya", "modern-bert.decision.max_head_tokens": float64(256)},
		trainingContext: 1024,
		reserved:        []string{"<bos>", "<eos>", "<mask>"},
	},
	d3Digest: {
		modelInfo:         map[string]any{"qwen35.decision.type": "vllm-d3"},
		quantization:      "Q8_0",
		configuredContext: 4096,
		reserved: []string{
			"<|endoftext|>", "<|im_start|>", "<|im_end|>", "<|object_ref_start|>",
			"<|object_ref_end|>", "<|box_start|>", "<|box_end|>", "<|quad_start|>",
			"<|quad_end|>", "<|vision_start|>", "<|vision_end|>", "<|vision_pad|>",
			"<|image_pad|>", "<|video_pad|>", "<tool_call>", "</tool_call>",
			"<|fim_prefix|>", "<|fim_middle|>", "<|fim_suffix|>", "<|fim_pad|>",
			"<|repo_name|>", "<|file_sep|>", "<tool_response>", "</tool_response>",
			"<think>", "</think>", "<|audio_start|>", "<|audio_end|>",
			"<tts_pad>", "<tts_text_bos>", "<tts_text_eod>", "<tts_text_bos_single>",
			"<|audio_pad|>",
		},
	},
}

// Ollama accepts namespaced names and explicit registry ports. Always require an explicit tag.
var modelTag = regexp.MustCompile(`^(?:[a-zA-Z0-9][a-zA-Z0-9._-]*:[0-9]+/)?(?:[a-zA-Z0-9][a-zA-Z0-9._-]*/)*[a-zA-Z0-9][a-zA-Z0-9._-]*:[a-zA-Z0-9_][a-zA-Z0-9._-]*$`)

type Adapter struct {
	model, digest string
	managedID     string
	measured      measuredProfile
	base          string
	client        *http.Client
	questions     map[string]question
	profile       string
}

func New(base string) (*Adapter, error) { return NewSystemOne(base, Model, Digest) }

// NewSystemOne configures one local decision model pinned to an installed Ollama manifest.
func NewSystemOne(base, model, digest string) (*Adapter, error) {
	a, err := newAdapter(base, model, digest, "")
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, errors.New("record classifier requires a local HTTP origin")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); host != "localhost" && host != "ollama" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("record classifier requires loopback or the local ollama service")
	}
	a.measured = measuredProfiles[a.digest]
	return a, nil
}

func newAdapter(base, model, digest, managedID string) (*Adapter, error) {
	digest = strings.TrimPrefix(digest, "sha256:")
	rawDigest, err := hex.DecodeString(digest)
	if err != nil || len(rawDigest) != sha256.Size {
		return nil, errors.New("record classifier requires a full SHA256 digest")
	}
	tag := strings.ToLower(model)
	if !modelTag.MatchString(model) || strings.HasSuffix(tag, ":cloud") || strings.HasSuffix(tag, "-cloud") {
		return nil, errors.New("record classifier requires an explicit local model:tag")
	}
	digest = hex.EncodeToString(rawDigest)
	var questions map[string]question
	if err := json.Unmarshal(flatQuestions, &questions); err != nil {
		return nil, err
	}
	runtime := "ollama0.40.16"
	if managedID != "" {
		runtime = "models-configurator-decisions-v1:" + managedID
	}
	hash := sha256.Sum256(append([]byte(model+":"+digest+":taxonomy2:"+runtime+":review-only-v1:"), flatQuestions...))
	return &Adapter{
		model: model, digest: digest, managedID: managedID, base: strings.TrimRight(base, "/"), questions: questions, profile: hex.EncodeToString(hash[:]),
		client: &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
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

func configuredContext(parameters string) int {
	limit := 0
	for _, line := range strings.Split(parameters, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "num_ctx" {
			continue
		}
		if len(fields) != 2 || limit != 0 {
			return 0
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n <= 0 {
			return 0
		}
		limit = n
	}
	return limit
}

// verifyRuntime checks identity before transmitting record text and returns the runtime's context limit.
func (a *Adapter) verifyRuntime(ctx context.Context) (int, error) {
	if err := a.verifyPin(ctx); err != nil {
		return 0, err
	}
	var version struct{ Version string }
	if err := a.request(ctx, "/api/version", nil, &version); err != nil {
		return 0, err
	}
	var show struct {
		Parameters string
		Details    struct {
			Format, Runner string
			Quantization   string `json:"quantization_level"`
		}
		Info         map[string]any `json:"model_info"`
		Capabilities []string
		RemoteModel  string `json:"remote_model"`
		RemoteHost   string `json:"remote_host"`
	}
	payload, _ := json.Marshal(map[string]string{"model": a.model})
	if err := a.request(ctx, "/api/show", payload, &show); err != nil {
		return 0, err
	}
	if version.Version != "0.40.16" || !slices.Contains(show.Capabilities, "decision") || show.RemoteHost != "" || show.RemoteModel != "" {
		return 0, errors.New("unsupported local decision runtime")
	}
	measured := a.measured
	if len(measured.modelInfo) > 0 && (show.Details.Format != "gguf" || show.Details.Runner != "llamacpp") {
		return 0, errors.New("unsupported measured model renderer")
	}
	for key, expected := range measured.modelInfo {
		if show.Info[key] != expected {
			return 0, errors.New("decision model metadata mismatch")
		}
	}
	if measured.quantization != "" && show.Details.Quantization != measured.quantization {
		return 0, errors.New("decision model quantization mismatch")
	}
	limit := configuredContext(show.Parameters)
	if measured.configuredContext != 0 && measured.configuredContext != limit {
		return 0, errors.New("decision model context differs from measured profile")
	}
	if measured.trainingContext != 0 {
		// The Laya checkpoint has a measured inclusive training bound independent of num_ctx.
		return 0, nil
	}
	if limit == 0 {
		return 0, errors.New("configured decision model num_ctx is required")
	}
	return limit, nil
}

type answer struct {
	Type, Choice  string
	Confidence    *float64
	Probabilities map[string]*float64
}

type usage struct {
	Input  *int `json:"input_tokens"`
	Output *int `json:"output_tokens"`
}

func validScore(v *float64) bool {
	return v != nil && *v >= 0 && *v <= 1 && !math.IsNaN(*v) && !math.IsInf(*v, 0)
}

func (a *Adapter) systemOneDecision(ctx context.Context, q question, source string) (answer, usage, string, error) {
	payload, err := json.Marshal(map[string]any{"model": a.model, "state": source, "questions": map[string]json.RawMessage{"classification": q.Wire}, "keep_alive": "5m"})
	if err != nil {
		return answer{}, usage{}, "", err
	}
	if len(payload) > maxBodyBytes {
		return answer{}, usage{}, "input_limit", nil
	}
	var res struct {
		Model   string
		Answers map[string]answer
		Usage   usage
	}
	if err = a.request(ctx, "/v1/systemone", payload, &res); err != nil {
		if err == runtimeError(http.StatusRequestEntityTooLarge) {
			return answer{}, usage{}, "input_limit", nil
		}
		return answer{}, usage{}, "", err
	}
	if err = a.verifyPin(ctx); err != nil {
		return answer{}, usage{}, "", err
	}
	if res.Model != a.model || len(res.Answers) != 1 {
		return answer{}, usage{}, "", errors.New("invalid single-record choice")
	}
	return res.Answers["classification"], res.Usage, "", nil
}

func (a *Adapter) axis(ctx context.Context, axis, source string, contextLimit int) ([]classification.Alternative, string, error) {
	q := a.questions[axis]
	decide := a.systemOneDecision
	if a.managedID != "" {
		decide = a.managedDecision
	}
	ans, tokens, reason, err := decide(ctx, q, source)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	if tokens.Input == nil || tokens.Output == nil || *tokens.Input < 0 || *tokens.Output < 0 || ans.Type != "choice" || q.Labels[ans.Choice] == "" || !validScore(ans.Confidence) || len(ans.Probabilities) != len(q.Labels) {
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
	if contextLimit > 0 && *tokens.Input >= contextLimit {
		return nil, "input_limit", nil
	}
	if a.measured.trainingContext > 0 && *tokens.Input > a.measured.trainingContext {
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
	for _, token := range a.measured.reserved {
		if strings.Contains(source, token) {
			result.Reason = "reserved_token"
			return result, nil
		}
	}
	if len(source) > maxBodyBytes {
		result.Reason = "input_limit"
		return result, nil
	}
	contextLimit := 0
	if a.managedID == "" {
		var err error
		contextLimit, err = a.verifyRuntime(ctx)
		if err != nil {
			return classification.Prediction{}, err
		}
	}
	for _, axis := range []string{"semantic", "representation"} {
		options, reason, err := a.axis(ctx, axis, source, contextLimit)
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
