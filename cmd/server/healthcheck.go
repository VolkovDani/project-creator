package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Readiness includes the configured token, GraphQL schema and a real DB read;
// models are explicitly prepared and checked separately.
func healthcheck(ctx context.Context, endpoint, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(`{"query":"{projects(limit:1,offset:0){id}}"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("backend not ready")
	}
	defer response.Body.Close()
	var result struct {
		Data struct {
			Projects *[]json.RawMessage `json:"projects"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil || len(result.Errors) != 0 || result.Data.Projects == nil {
		return errors.New("backend not ready")
	}
	return nil
}
