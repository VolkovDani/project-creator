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
	cacheSize        = 64
	callsPerMinute   = 6
	inferenceTimeout = 90 * time.Second
)

var rules = contracts.ClassificationRules()

type cacheKey struct {
	workspace, taxonomy, profile string
	request                      Request
	source                       [sha256.Size]byte
}
type cacheEntry struct {
	key    cacheKey
	result Result
}
type Service struct {
	db        *sql.DB
	generator Generator
	mu        sync.Mutex
	cache     []cacheEntry
	recent    map[string][]time.Time
	running   bool
}

func New(db *sql.DB, g Generator) *Service {
	return &Service{db: db, generator: g, recent: make(map[string][]time.Time)}
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
	if cached, status := s.acquire(key, time.Now()); status != "" {
		return cached, status, nil, nil
	}
	defer s.release()
	call, cancel := context.WithTimeout(ctx, inferenceTimeout)
	defer cancel()
	result, err := s.generator.ExplainClassification(call, source, req.Target)
	if call.Err() != nil {
		return nil, failureStatus(call.Err()), nil, nil
	}
	if err != nil {
		return nil, failureStatus(err), nil, nil
	}

	// The model can outlive an edit, archive or same-version reclassification.
	current, status, err := s.source(call, workspace, req)
	if err != nil || status != "ready" {
		return nil, status, nil, err
	}
	currentData, err := json.Marshal(current)
	if err != nil {
		return nil, "", nil, err
	}
	if sha256.Sum256(currentData) != key.source || s.generator.ExplanationProfile() != profile {
		return nil, "stale", nil, nil
	}
	s.store(key, result)
	return &result, "ready", nil, nil
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

func cloneResult(result Result) *Result {
	result.Meaning.Criteria = slices.Clone(result.Meaning.Criteria)
	result.Form.Criteria = slices.Clone(result.Form.Criteria)
	result.Questions = slices.Clone(result.Questions)
	return &result
}

// Only one interactive call can occupy the local generator. Cache hits need
// neither a slot nor a rate-limit allowance. Expired workspace windows are removed.
func (s *Service) acquire(key cacheKey, now time.Time) (*Result, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.cache {
		if entry.key == key {
			return cloneResult(entry.result), "ready"
		}
	}
	if s.running {
		return nil, "busy"
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
	if len(s.recent[key.workspace]) >= callsPerMinute {
		return nil, "busy"
	}
	s.recent[key.workspace] = append(s.recent[key.workspace], now)
	s.running = true
	return nil, ""
}
func (s *Service) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}
func (s *Service) store(key cacheKey, result Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) == cacheSize {
		copy(s.cache, s.cache[1:])
		s.cache = s.cache[:cacheSize-1]
	}
	s.cache = append(s.cache, cacheEntry{key, *cloneResult(result)})
}
