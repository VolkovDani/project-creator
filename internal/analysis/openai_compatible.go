package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"

	"github.com/VolkovDani/project-creator/internal/oaicompat"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

// OpenAICompatible compares requirements using the configured Chat Completions endpoint.
type OpenAICompatible struct {
	client              *oaicompat.Client
	schema              map[string]any
	profile, provenance string
	managedNPU          bool
	localManagedNPU     bool
}

const chatCompletionTokens = 1024

// Models Configurator IDs consist of the provider, configured name and SHA-256 identity.
var managedNPUModelID = regexp.MustCompile(`^openvino-npu\.[A-Za-z0-9][A-Za-z0-9_.-]{0,79}\.[0-9a-f]{64}$`)
var managedChatModelID = regexp.MustCompile(`^(openvino-npu|ollama|openai-compatible)\.[A-Za-z0-9][A-Za-z0-9_.-]{0,79}\.[0-9a-f]{64}$`)

// Use the strict schema subset shared by compatible servers. The local decoders
// still enforce the original length, version and source constraints.
func compatibleSchema(schema map[string]any) map[string]any {
	for _, property := range schema["properties"].(map[string]any) {
		constraints := property.(map[string]any)
		for _, key := range []string{"minLength", "maxLength", "minimum", "maximum"} {
			delete(constraints, key)
		}
	}
	return schema
}

func compatibleProvenance(client *oaicompat.Client, adapter string) generationProvenance {
	return generationProvenance{
		Adapter:               adapter,
		Model:                 client.Model(),
		EndpointModelIdentity: client.Identity(),
		Parameters:            map[string]any{"max_completion_tokens": chatCompletionTokens, "stream": false, "responseFormat": "json_schema", "strict": true},
	}
}

func NewOpenAICompatible(baseURL, model, key string) (*OpenAICompatible, error) {
	client, err := oaicompat.New(baseURL, model, key)
	if err != nil {
		return nil, err
	}
	if managedNPUModelID.MatchString(model) {
		client = client.WithErrorCodes(func(status int, code string) error {
			if npuFailureStatus(code) == status {
				return npuFailure(code)
			}
			return nil
		})
		spec := compatibleProvenance(client, "openai-compatible-managed-npu-pair-v1")
		spec.Parameters["userMessageEncoding"] = "python-json-ensure-ascii-false-v1"
		provenance, profile := npuPairProvenance(spec)
		return &OpenAICompatible{
			client: client, profile: profile, provenance: provenance,
			managedNPU: true, localManagedNPU: localModelManagerURL(baseURL),
		}, nil
	}
	spec := compatibleProvenance(client, "openai-compatible-pair-v1")
	spec.ResponseSchema = compatibleSchema(findingSchema())
	spec.SystemPrompt = systemPrompt
	provenance, profile := encodePairProvenance(spec, alphabetRetryInstruction)
	return &OpenAICompatible{client: client, schema: spec.ResponseSchema, profile: profile, provenance: provenance}, nil
}

func localModelManagerURL(base string) bool {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || (u.EscapedPath() != "/v1" && u.EscapedPath() != "/v1/") {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback() ||
		(host == "models-configurator" && u.Port() == "8317")
}

func (g *OpenAICompatible) Profile() string    { return g.profile }
func (g *OpenAICompatible) Provenance() string { return g.provenance }

func (g *OpenAICompatible) Compare(ctx context.Context, pair Pair) (Finding, error) {
	if !validSource(pair.Left) || !validSource(pair.Right) || pair.Left.ID == pair.Right.ID {
		return Finding{}, errors.New("invalid comparison input")
	}
	if g.managedNPU {
		return compareNPUPair(ctx, pair, g.generateNPUFinding)
	}
	data, err := json.Marshal(pair)
	if err != nil {
		return Finding{}, errors.New("invalid comparison input")
	}
	return generatePairFinding(ctx, pair, alphabetRetryInstruction, func(prompt string, _ bool) (Finding, error) {
		return g.generateFinding(ctx, pair, prompt, string(data), g.schema)
	})
}

func (g *OpenAICompatible) generateNPUFinding(ctx context.Context, aliases Pair, prompt string, schema map[string]any) (Finding, error) {
	user, err := npuUserMessage(aliases)
	if err != nil {
		return Finding{}, err
	}
	return g.generateFinding(ctx, aliases, prompt, user, schema)
}

func (g *OpenAICompatible) generateFinding(ctx context.Context, pair Pair, prompt, user string, schema map[string]any) (Finding, error) {
	messages := []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": user}}
	content, err := compatibleChat(ctx, g.client, messages, "requirement_finding", schema)
	if err != nil {
		return Finding{}, err
	}
	return decodeFinding(content, pair)
}

// SummaryOpenAICompatible generates scoped previews using Chat Completions.
type SummaryOpenAICompatible struct {
	*chatSummary
	budget *oaicompat.Client
}

func NewSummaryOpenAICompatible(baseURL, model, key, allowedLanguages string) (*SummaryOpenAICompatible, error) {
	client, err := oaicompat.New(baseURL, model, key)
	if err != nil {
		return nil, err
	}
	spec := compatibleProvenance(client, "openai-compatible-summary-v1")
	managedNPU := managedNPUModelID.MatchString(model)
	var budgetClient *oaicompat.Client
	if localModelManagerURL(baseURL) && managedChatModelID.MatchString(model) {
		budgetClient = client.WithErrorCodes(func(status int, code string) error {
			if status == 413 && code == "TOKEN_COUNT_REQUEST_TOO_LARGE" {
				return processgraph.ErrBatchTooLarge
			}
			return nil
		})
	}
	if managedNPU {
		// The runtime can report an overlong prompt with this same sanitized code.
		client = client.WithErrorCodes(func(status int, code string) error {
			if code == "OUTPUT_TOKEN_LIMIT" && status == http.StatusBadGateway {
				return errChatOutputLimit
			}
			if code == "NPU_UNAVAILABLE" && npuFailureStatus(code) == status {
				return npuFailure(code)
			}
			return nil
		})
	}
	schemaFor := func(pattern string) map[string]any {
		schema := compatibleSchema(summaryResponseSchema(pattern))
		if managedNPU && pattern != "" {
			// OpenVINO can append an empty extra key with a patterned string,
			// even when required and additionalProperties define a single key.
			schema["minProperties"] = 1
			schema["maxProperties"] = 1
		}
		return schema
	}
	summary, err := newChatSummary(allowedLanguages, spec, schemaFor, context.Context.Err, func(ctx context.Context, messages []map[string]string, schema map[string]any) (string, error) {
		return compatibleChat(ctx, client, messages, summaryChatSchemaName, schema)
	})
	if err != nil {
		return nil, err
	}
	if managedNPU {
		summary.managedNPU = true
		summary.outputValidation += "+managed-npu-language-repair-exactly-one-property+ordered-source-number-retry-v1"
	}
	return &SummaryOpenAICompatible{chatSummary: summary, budget: budgetClient}, nil
}

const summaryChatSchemaName = "requirement_summary"

func compatibleChatBody(client *oaicompat.Client, messages []map[string]string, name string, schema map[string]any) map[string]any {
	return map[string]any{
		"model": client.Model(), "messages": messages, "stream": false, "max_completion_tokens": chatCompletionTokens,
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": name, "strict": true, "schema": schema}},
	}
}

type compatibleChatBudget struct {
	Model               string `json:"model"`
	InputTokens         int    `json:"input_tokens"`
	MaxInputTokens      int    `json:"max_input_tokens"`
	MaxCompletionTokens int    `json:"max_completion_tokens"`
	Exact               *bool  `json:"exact"`
}

func readCompatibleChatBudget(ctx context.Context, client *oaicompat.Client, body map[string]any) (compatibleChatBudget, error) {
	var budget compatibleChatBudget
	if err := client.Post(ctx, "/chat/token-count", body, &budget); err != nil {
		return compatibleChatBudget{}, err
	}
	if budget.Model != client.Model() || budget.InputTokens < 1 || budget.MaxInputTokens < 1 ||
		budget.MaxCompletionTokens < 1 || budget.Exact == nil {
		return compatibleChatBudget{}, errors.New("invalid chat token budget")
	}
	if cap, ok := body["max_completion_tokens"].(int); ok && budget.MaxCompletionTokens > cap {
		return compatibleChatBudget{}, errors.New("invalid chat completion budget")
	}
	return budget, nil
}

func compatibleChat(ctx context.Context, client *oaicompat.Client, messages []map[string]string, name string, schema map[string]any) (string, error) {
	return completeCompatibleChat(ctx, client, compatibleChatBody(client, messages, name, schema))
}

func completeCompatibleChat(ctx context.Context, client *oaicompat.Client, body map[string]any) (string, error) {
	var response struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role         string            `json:"role"`
				Content      *string           `json:"content"`
				Refusal      *string           `json:"refusal"`
				ToolCalls    []json.RawMessage `json:"tool_calls"`
				FunctionCall *struct{}         `json:"function_call"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := client.Post(ctx, "/chat/completions", body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 1 && response.Choices[0].FinishReason == "length" {
		return "", errChatOutputLimit
	}
	if len(response.Choices) != 1 || response.Choices[0].FinishReason != "stop" {
		return "", invalidModelOutput("incomplete chat completion")
	}
	message := response.Choices[0].Message
	if message.Role != "assistant" || message.Content == nil || message.Refusal != nil || len(message.ToolCalls) > 0 || message.FunctionCall != nil {
		return "", invalidModelOutput("unsupported chat completion message")
	}
	return *message.Content, nil
}
