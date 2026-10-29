package analysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// OpenVINO talks only to an explicitly selected local Unix socket. The runtime
// independently pins model artifacts and refuses unavailable NPU execution.
type OpenVINO struct {
	pinnedProfile, profile string
	client                 *http.Client
}

var npuProfilePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func NewOpenVINO(socket, profile string) (*OpenVINO, error) {
	if !filepath.IsAbs(socket) || strings.ContainsAny(socket, "\x00\r\n") || !npuProfilePattern.MatchString(profile) {
		return nil, errors.New("OpenVINO requires an absolute Unix socket path and full lowercase SHA256 profile")
	}
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 1, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	sum := sha256.Sum256([]byte("openvino-npu-unix-v1\ncomparison-prompt-v1:2000\n" + profile))
	return &OpenVINO{pinnedProfile: profile, profile: hex.EncodeToString(sum[:]), client: &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects disabled") }}}, nil
}

func (o *OpenVINO) Profile() string { return o.profile }

func (o *OpenVINO) request(ctx context.Context, method, path string, body []byte, out any) error {
	request, err := http.NewRequestWithContext(ctx, method, "http://npu.local"+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid NPU request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := o.client.Do(request)
	if err != nil {
		return errors.New("local NPU runtime unavailable or request cancelled")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("local NPU runtime rejected request; inspect runtime diagnostics")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return errors.New("invalid NPU response size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return errors.New("invalid NPU response")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("NPU response contains trailing data")
	}
	return nil
}

func (o *OpenVINO) verify(ctx context.Context) error {
	var health struct {
		Profile string `json:"profile"`
		Device  string `json:"device"`
	}
	if err := o.request(ctx, "GET", "/health", nil, &health); err != nil {
		return err
	}
	return o.verifyIdentity(health.Profile, health.Device)
}

func (o *OpenVINO) verifyIdentity(profile, device string) error {
	if profile != o.pinnedProfile || device != "NPU" {
		return errors.New("NPU runtime profile or execution device differs from pinned configuration")
	}
	return nil
}

func (o *OpenVINO) Compare(ctx context.Context, pair Pair) (Finding, error) {
	if !validSource(pair.Left) || !validSource(pair.Right) || pair.Left.ID == pair.Right.ID {
		return Finding{}, errors.New("invalid comparison input")
	}
	if err := o.verify(ctx); err != nil {
		return Finding{}, err
	}
	body, err := json.Marshal(map[string]any{"profile": o.pinnedProfile, "system": systemPrompt, "pair": pair, "schema": findingSchema()})
	if err != nil {
		return Finding{}, errors.New("invalid NPU comparison request")
	}
	var response struct {
		Profile string          `json:"profile"`
		Device  string          `json:"device"`
		Finding json.RawMessage `json:"finding"`
	}
	if err := o.request(ctx, "POST", "/compare", body, &response); err != nil {
		return Finding{}, err
	}
	if err := o.verifyIdentity(response.Profile, response.Device); err != nil {
		return Finding{}, err
	}
	finding, err := decodeFinding(string(response.Finding), pair)
	if err != nil {
		return Finding{}, err
	}
	if err := o.verify(ctx); err != nil {
		return Finding{}, err
	}
	if ctx.Err() != nil {
		return Finding{}, ctx.Err()
	}
	return finding, nil
}
