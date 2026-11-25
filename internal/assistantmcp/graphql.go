package assistantmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const proposalFields = `id analysis limitations client model sharing status createdAt sources { requirementId version status } glossarySources { termId version status }`

type pageInfo struct {
	EndCursor   *string `json:"endCursor,omitempty"`
	HasNextPage bool    `json:"hasNextPage"`
}

type requirement struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Version int32  `json:"version"`
}

type requirementPage struct {
	Items    []requirement `json:"items"`
	PageInfo pageInfo      `json:"pageInfo"`
}

type proposalPage struct {
	Items    []proposal `json:"items"`
	PageInfo pageInfo   `json:"pageInfo"`
}

type proposalSource struct {
	RequirementID string `json:"requirementId" jsonschema:"ID returned by list_requirements."`
	Version       int32  `json:"version" jsonschema:"Version of the requirement you read."`
}

type reviewedSource struct {
	proposalSource
	Status string `json:"status"`
}

type glossarySource struct {
	TermID  string `json:"termId" jsonschema:"Confirmed term ID returned in a pair's glossaryContext."`
	Version int32  `json:"version" jsonschema:"Version of the confirmed term you read."`
}

type reviewedGlossarySource struct {
	glossarySource
	Status string `json:"status"`
}

type proposal struct {
	ID              string                   `json:"id"`
	Analysis        string                   `json:"analysis"`
	Limitations     string                   `json:"limitations"`
	Client          string                   `json:"client"`
	Model           string                   `json:"model"`
	Sharing         string                   `json:"sharing"`
	Status          string                   `json:"status"`
	CreatedAt       string                   `json:"createdAt"`
	Sources         []reviewedSource         `json:"sources"`
	GlossarySources []reviewedGlossarySource `json:"glossarySources"`
}

type saveInput struct {
	RequestID       string           `json:"requestId" jsonschema:"Your unique ID for this proposal. Keep it unchanged when retrying."`
	Analysis        string           `json:"analysis" jsonschema:"Your proposed analysis and its reasoning, up to 10000 characters."`
	Limitations     string           `json:"limitations" jsonschema:"Scope limits or uncertainties of the proposal, up to 4000 characters."`
	Model           string           `json:"model" jsonschema:"Your model identifier, or unknown if unavailable."`
	Sources         []proposalSource `json:"sources" jsonschema:"Cite 1 to 20 distinct requirements and their versions."`
	GlossarySources []glossarySource `json:"glossarySources,omitempty" jsonschema:"Cite up to 20 confirmed glossary term IDs and versions used in the analysis."`
}

type errorParam struct {
	Key   string `json:"key"`
	Value int32  `json:"value"`
}

type fieldError struct {
	Path       []string     `json:"path"`
	Code       string       `json:"code"`
	MessageKey string       `json:"messageKey"`
	Params     []errorParam `json:"params"`
}

type saveResult struct {
	Proposal    *proposal    `json:"proposal,omitempty"`
	FieldErrors []fieldError `json:"fieldErrors"`
}

type backend struct {
	cfg  Config
	http *http.Client
}

func (b backend) query(ctx context.Context, query string, variables map[string]any, data any) error {
	body, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{Query: query, Variables: variables})
	if err != nil {
		return errors.New("invalid GraphQL request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.cfg.endpoint(), bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid local GraphQL address")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.cfg.Token)
	resp, err := b.http.Do(req)
	if err != nil {
		return errors.New("local GraphQL API is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("local GraphQL API rejected the workspace token")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New("local GraphQL request failed")
	}
	const maxResponseBytes = 4 << 20
	response, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(response) > maxResponseBytes {
		return errors.New("invalid local GraphQL response")
	}
	var result struct {
		Data   json.RawMessage
		Errors []json.RawMessage
	}
	if json.Unmarshal(response, &result) != nil || len(result.Data) == 0 || bytes.Equal(result.Data, []byte("null")) || len(result.Errors) != 0 ||
		json.Unmarshal(result.Data, data) != nil {
		return errors.New("project unavailable or local GraphQL request failed")
	}
	return nil
}
