package analysis

import (
	"context"
	"errors"
	"time"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

const processGraphPreflightTimeout = 2 * time.Minute

func (s *SummaryOpenAICompatible) PlanProcessGraphWithContext(ctx context.Context, req processgraph.Request) (processgraph.BatchPlan, error) {
	if s.budget == nil {
		return s.PlanProcessGraph(req)
	}
	output, err := s.processGraphOutput()
	if err != nil {
		return processgraph.BatchPlan{}, err
	}
	matchGlossary := processGraphGlossaryMatches(req)
	measure, cancel := context.WithTimeout(ctx, processGraphPreflightTimeout)
	defer cancel()
	plan, err := planProcessGraph(req, func(batch processgraph.Request) (bool, error) {
		prompt := s.processGraphPrompt(batch, output.edgeLimit)
		input, err := renderProcessGraph(batch, matchGlossary, prompt, 0, false, nil)
		if err != nil {
			return false, err
		}
		return s.graphPromptFits(measure, input.messages, output.schema)
	})
	if ctx.Err() != nil {
		return processgraph.BatchPlan{}, ctx.Err()
	}
	if err != nil {
		// Older managers and profiles without a tokenizer keep the existing
		// conservative plan. Use one sizing method for the whole run.
		return s.PlanProcessGraph(req)
	}
	return plan, nil
}

func (s *SummaryOpenAICompatible) graphPromptFits(ctx context.Context, messages []map[string]string, schema map[string]any) (bool, error) {
	body := compatibleChatBody(s.budget, messages, summaryChatSchemaName, schema)
	var budget struct {
		Model               string `json:"model"`
		InputTokens         int    `json:"input_tokens"`
		MaxInputTokens      int    `json:"max_input_tokens"`
		MaxCompletionTokens int    `json:"max_completion_tokens"`
		Exact               *bool  `json:"exact"`
	}
	err := s.budget.Post(ctx, "/chat/token-count", body, &budget)
	if errors.Is(err, processgraph.ErrBatchTooLarge) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if budget.Model != s.budget.Model() || budget.InputTokens < 1 || budget.MaxInputTokens < 1 ||
		budget.MaxCompletionTokens < 1 || budget.MaxCompletionTokens > chatCompletionTokens || budget.Exact == nil {
		return false, errors.New("invalid chat token budget")
	}
	return budget.InputTokens <= budget.MaxInputTokens, nil
}

func (s *SummaryOpenAICompatible) GenerateProcessGraphBatch(ctx context.Context, req processgraph.Request) (processgraph.Suggestion, error) {
	// Both the preflight and conservative plans already bounded these batches.
	return s.generateProcessGraphBatch(ctx, req, s.budget != nil)
}
