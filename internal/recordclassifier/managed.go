package recordclassifier

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// The decision API currently exposes only pinned local Ollama profiles.
var managedProfileID = regexp.MustCompile(`^ollama\.[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}\.[a-f0-9]{64}$`)

// NewManaged uses the manager's immutable ID for routing and the artifact identity for review records.
func NewManaged(base, profileID, model, digest string) (*Adapter, error) {
	if !managedProfileID.MatchString(profileID) {
		return nil, errors.New("managed record classifier requires a full local decision profile ID")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(base, "#") ||
		(u.EscapedPath() != "/v1" && u.EscapedPath() != "/v1/") {
		return nil, errors.New("managed record classifier requires an explicit local HTTP URL ending in /v1")
	}
	host := u.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() && !(host == "models-configurator" && u.Port() == "8317") {
		return nil, errors.New("managed record classifier requires loopback or models-configurator:8317")
	}
	return newAdapter(base, model, digest, profileID)
}

func (a *Adapter) managedDecision(ctx context.Context, q question, source string) (answer, usage, string, error) {
	payload, err := json.Marshal(map[string]any{"model": a.managedID, "digest": a.digest, "state": source, "question": q.Wire})
	if err != nil {
		return answer{}, usage{}, "", err
	}
	var res struct {
		Model, Digest, Reason string
		Answer                *answer
		Usage                 *usage
	}
	if err = a.request(ctx, "/decisions", payload, &res); err != nil {
		return answer{}, usage{}, "", err
	}
	if res.Model != a.managedID || res.Digest != a.digest {
		return answer{}, usage{}, "", errors.New("managed record classifier profile binding mismatch")
	}
	if res.Reason == "" && res.Answer != nil && res.Usage != nil {
		return *res.Answer, *res.Usage, "", nil
	}
	if res.Answer == nil && res.Usage == nil {
		switch res.Reason {
		case "reserved_token", "input_limit", "outside_training_context":
			return answer{}, usage{}, res.Reason, nil
		}
	}
	return answer{}, usage{}, "", errors.New("invalid managed record classifier response")
}
