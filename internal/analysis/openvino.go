package analysis

import (
	"bytes"
	"context"
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
	pinnedProfile, profile, provenance string
	client                             *http.Client
}

var npuProfilePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func NewOpenVINO(socket, profile string) (*OpenVINO, error) {
	if !filepath.IsAbs(socket) || strings.ContainsAny(socket, "\x00\r\n") || !npuProfilePattern.MatchString(profile) {
		return nil, errors.New("OpenVINO requires an absolute Unix socket path and full lowercase SHA256 profile")
	}
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 1, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	provenance, key := encodeGenerationProvenance(generationProvenance{Adapter: "openvino-npu-unix-v1", RuntimeProfile: profile, SystemPrompt: systemPrompt, ResponseSchema: findingSchema(), Normalization: "source-aliases-v2:text-order"})
	return &OpenVINO{pinnedProfile: profile, profile: key, provenance: provenance, client: &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects disabled") }}}, nil
}

func (o *OpenVINO) Profile() string    { return o.profile }
func (o *OpenVINO) Provenance() string { return o.provenance }

func (o *OpenVINO) request(ctx context.Context, method, path string, body []byte, out any) error {
	request, err := http.NewRequestWithContext(ctx, method, "http://npu.local"+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid NPU request")
	}
	request.Header.Set("Content-Type", "application/json")
	if call, ok := ctx.Value(npuCallKey{}).(npuCall); ok && callIDPattern.MatchString(call.CallID) {
		request.Header.Set("X-NPU-Call-ID", call.CallID)
	}
	response, err := o.client.Do(request)
	if err != nil {
		return npuFailure("NPU_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Code string `json:"code"`
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 4097))
		if readErr == nil && len(data) <= 4096 && json.Unmarshal(data, &failure) == nil && knownNPUFailure(failure.Code) {
			return npuFailure(failure.Code)
		}
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
		return npuFailure("PROFILE_CHANGED")
	}
	return nil
}

func (o *OpenVINO) Compare(ctx context.Context, pair Pair) (Finding, error) {
	call, _ := ctx.Value(npuCallKey{}).(npuCall)
	if !callIDPattern.MatchString(call.CallID) {
		id, err := identifier()
		if err != nil {
			return Finding{}, errors.New("NPU call identifier unavailable")
		}
		call.CallID = id
	}
	ctx = context.WithValue(ctx, npuCallKey{}, call)
	path := npuLogPath()
	writeNPUCall(path, "compare_started", call)
	defer writeNPUCall(path, "compare_returned", call)

	if !validSource(pair.Left) || !validSource(pair.Right) || pair.Left.ID == pair.Right.ID {
		return Finding{}, errors.New("invalid comparison input")
	}
	if err := o.verify(ctx); err != nil {
		return Finding{}, err
	}
	// Opaque IDs and revision numbers are references, not requirement semantics.
	// Keep their per-request mapping in the adapter so incidental identifiers do
	// not influence the model's verdict. Source texts remain byte-for-byte intact.
	aliases := Pair{
		Left:  Source{ID: "left", Version: 1, Text: pair.Left.Text},
		Right: Source{ID: "right", Version: 1, Text: pair.Right.Text},
	}
	// Contradiction is symmetric. Canonical text order keeps the same comparison
	// independent of the caller's source order while preserving every source byte.
	if aliases.Right.Text < aliases.Left.Text {
		aliases.Left.Text, aliases.Right.Text = aliases.Right.Text, aliases.Left.Text
	}
	body, err := json.Marshal(map[string]any{"profile": o.pinnedProfile, "system": systemPrompt, "pair": aliases, "schema": findingSchema()})
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
	finding, err := decodeFinding(string(response.Finding), aliases)
	if err != nil {
		return Finding{}, err
	}
	finding.LeftID, finding.LeftVersion = pair.Left.ID, pair.Left.Version
	finding.RightID, finding.RightVersion = pair.Right.ID, pair.Right.Version
	if !validFinding(pair, finding) {
		return Finding{}, errors.New("invalid NPU comparison source mapping")
	}
	if err := o.verify(ctx); err != nil {
		return Finding{}, err
	}
	if ctx.Err() != nil {
		return Finding{}, ctx.Err()
	}
	return finding, nil
}

// npuFailure carries only a closed set of transport diagnostics. Raw runtime
// errors may include input details and must never become API error codes.
type npuFailure string

func (e npuFailure) Error() string { return string(e) }
func knownNPUFailure(code string) bool {
	switch code {
	case "NPU_UNAVAILABLE", "NPU_RECOVERY_FAILED", "NPU_WORKER_EXIT", "TIMEOUT", "INVALID_OUTPUT", "PROFILE_CHANGED":
		return true
	}
	return false
}
