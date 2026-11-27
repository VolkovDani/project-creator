package explanation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/project"
)

const (
	callsPerMinute = 6
	// Cover both sequential model calls within the frontend's 190-second deadline.
	inferenceTimeout = 180 * time.Second
)

var rules = contracts.ClassificationRules()

type cacheKey struct {
	workspace, taxonomy, profile string
	request                      Request
	source                       [sha256.Size]byte
}
type Service struct {
	db        *sql.DB
	generator Generator
	cacheTTL  time.Duration
	mu        sync.Mutex
	recent    map[string][]time.Time
	running   bool
}

func New(db *sql.DB, g Generator) *Service {
	return NewWithCacheTTL(db, g, 0)
}

func NewWithCacheTTL(db *sql.DB, g Generator, ttl time.Duration) *Service {
	return &Service{db: db, generator: g, cacheTTL: ttl, recent: make(map[string][]time.Time)}
}

func validate(target Target) []project.FieldError {
	var fields []project.FieldError
	invalid := func(path, key string) {
		fields = append(fields, project.FieldError{Path: []string{"input", path}, Code: "INVALID_VALUE", MessageKey: key, Params: map[string]int{}})
	}
	if !slices.Contains(rules.Semantic, target.Semantic) {
		invalid("semanticType", "classification.invalidType")
	}
	if !slices.Contains(rules.Representation, target.Representation) {
		invalid("representationType", "classification.invalidType")
	}
	if target.Locale != "en" && target.Locale != "ru" {
		invalid("locale", "explanation.invalidLocale")
	}
	return fields
}

func (s *Service) Explain(ctx context.Context, workspace string, req Request) (*Result, string, []project.FieldError, error) {
	if fields := validate(req.Target); len(fields) > 0 {
		return nil, "invalid", fields, nil
	}
	source, status, err := s.source(ctx, workspace, req)
	if err != nil || status != "ready" {
		return nil, status, nil, err
	}
	if s.generator == nil {
		return nil, "unavailable", nil, nil
	}
	profile := s.generator.ExplanationProfile()
	if profile == "" {
		return nil, "unavailable", nil, nil
	}
	data, err := json.Marshal(source)
	if err != nil {
		return nil, "", nil, err
	}
	key := cacheKey{workspace: workspace, taxonomy: rules.Version, profile: profile, request: req, source: sha256.Sum256(data)}
	return s.explainCachedAxes(ctx, key, source)
}

// The model can outlive an edit, archive or same-version reclassification.
func (s *Service) checkCurrent(ctx context.Context, key cacheKey) (string, error) {
	current, status, err := s.source(ctx, key.workspace, key.request)
	if err != nil || status != "ready" {
		return status, err
	}
	currentData, err := json.Marshal(current)
	if err != nil {
		return "", err
	}
	if sha256.Sum256(currentData) != key.source || s.generator.ExplanationProfile() != key.profile {
		return "stale", nil
	}
	return "ready", nil
}

func failureStatus(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrInputTooLong):
		return "input_too_long"
	case errors.Is(err, ErrInvalidOutput):
		return "invalid_output"
	default:
		return "unavailable"
	}
}

// Only one interactive call can occupy the local generator. Cache hits need
// neither a slot nor a rate-limit allowance. Expired workspace windows are removed.
func (s *Service) admitLocked(workspace string, now time.Time) string {
	if s.running {
		return "busy"
	}
	cutoff := now.Add(-time.Minute)
	for w, times := range s.recent {
		for len(times) > 0 && !times[0].After(cutoff) {
			times = times[1:]
		}
		if len(times) == 0 {
			delete(s.recent, w)
		} else {
			s.recent[w] = times
		}
	}
	if len(s.recent[workspace]) >= callsPerMinute {
		return "busy"
	}
	s.recent[workspace] = append(s.recent[workspace], now)
	s.running = true
	return ""
}
func (s *Service) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}
