// Package oaicompat connects to an explicitly configured OpenAI-compatible API.
package oaicompat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base, model, key, identity string
	http                       *http.Client
	mapErrorCode               func(int, string) error
}

func New(baseURL, model, key string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(baseURL, "#") {
		return nil, errors.New("configure an HTTP(S) provider base URL without embedded credentials, query or fragment")
	}
	if model == "" || strings.TrimSpace(model) != model || len(model) > 200 || strings.ContainsAny(model, "\r\n") {
		return nil, errors.New("configure a valid provider model ID")
	}
	if strings.TrimSpace(key) != key || strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("invalid provider API key")
	}
	base := strings.TrimRight(u.String(), "/")
	digest := sha256.Sum256([]byte(base + "\n" + model))
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		base: base, model: model, key: key, identity: hex.EncodeToString(digest[:]),
		http: &http.Client{
			Transport:     transport,
			Timeout:       120 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) Model() string { return c.model }

// Identity distinguishes deployments that serve different weights under the same model ID.
func (c *Client) Identity() string { return c.identity }

// WithErrorCodes returns a client that recognizes selected status/code pairs.
// The mapper must return nil for unknown codes and only return sanitized errors.
func (c Client) WithErrorCodes(mapCode func(status int, code string) error) *Client {
	c.mapErrorCode = mapCode
	return &c
}

func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("invalid model request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return errors.New("invalid model request")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("model endpoint unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if c.mapErrorCode != nil {
			const limit = 4096
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
			if err == nil && len(raw) <= limit && json.Unmarshal(raw, &envelope) == nil {
				if mapped := c.mapErrorCode(resp.StatusCode, envelope.Error.Code); mapped != nil {
					return mapped
				}
			}
		}
		return fmt.Errorf("model endpoint returned HTTP %d", resp.StatusCode)
	}
	const maxResponse = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse || json.Unmarshal(data, out) != nil {
		return errors.New("invalid model response")
	}
	return nil
}
