package analysis

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/VolkovDani/project-creator/internal/oaicompat"
)

// OpenAICompatible compares requirements using the configured Chat Completions endpoint.
type OpenAICompatible struct {
	client              *oaicompat.Client
	schema              map[string]any
	profile, provenance string
}

const chatCompletionTokens = 1024

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
	spec := compatibleProvenance(client, "openai-compatible-pair-v1")
	spec.ResponseSchema = compatibleSchema(findingSchema())
	spec.SystemPrompt = systemPrompt
	provenance, profile := encodePairProvenance(spec)
	return &OpenAICompatible{client: client, schema: spec.ResponseSchema, profile: profile, provenance: provenance}, nil
}

func (g *OpenAICompatible) Profile() string    { return g.profile }
func (g *OpenAICompatible) Provenance() string { return g.provenance }

func (g *OpenAICompatible) Compare(ctx context.Context, pair Pair) (Finding, error) {
	if !validSource(pair.Left) || !validSource(pair.Right) || pair.Left.ID == pair.Right.ID {
		return Finding{}, errors.New("invalid comparison input")
	}
	data, err := json.Marshal(pair)
	if err != nil {
		return Finding{}, errors.New("invalid comparison input")
	}
	return generatePairFinding(ctx, func(prompt string) (Finding, error) {
		messages := []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": string(data)}}
		content, err := compatibleChat(ctx, g.client, messages, "requirement_finding", g.schema)
		if err != nil {
			return Finding{}, err
		}
		return decodeFinding(content, pair)
	})
}

// SummaryOpenAICompatible generates scoped previews using Chat Completions.
type SummaryOpenAICompatible struct{ *chatSummary }

func NewSummaryOpenAICompatible(baseURL, model, key, allowedLanguages string) (*SummaryOpenAICompatible, error) {
	client, err := oaicompat.New(baseURL, model, key)
	if err != nil {
		return nil, err
	}
	spec := compatibleProvenance(client, "openai-compatible-summary-v1")
	schemaFor := func(pattern string) map[string]any { return compatibleSchema(summaryResponseSchema(pattern)) }
	summary, err := newChatSummary(allowedLanguages, spec, schemaFor, context.Context.Err, func(ctx context.Context, messages []map[string]string, schema map[string]any) (string, error) {
		return compatibleChat(ctx, client, messages, "requirement_summary", schema)
	})
	if err != nil {
		return nil, err
	}
	return &SummaryOpenAICompatible{summary}, nil
}

func compatibleChat(ctx context.Context, client *oaicompat.Client, messages []map[string]string, name string, schema map[string]any) (string, error) {
	body := map[string]any{
		"model": client.Model(), "messages": messages, "stream": false, "max_completion_tokens": chatCompletionTokens,
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": name, "strict": true, "schema": schema}},
	}
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
	if len(response.Choices) != 1 || response.Choices[0].FinishReason != "stop" {
		return "", errors.New("incomplete chat completion")
	}
	message := response.Choices[0].Message
	if message.Role != "assistant" || message.Content == nil || message.Refusal != nil || len(message.ToolCalls) > 0 || message.FunctionCall != nil {
		return "", errors.New("unsupported chat completion message")
	}
	return *message.Content, nil
}
