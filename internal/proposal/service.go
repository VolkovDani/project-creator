// Package proposal owns drafts produced by external assistants and their source provenance.
package proposal

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/project"
)

const PendingReview = "pending_review"

var (
	ErrUnavailable   = errors.New("project unavailable")
	errSourceChanged = errors.New("source changed or unavailable")
	errRequestReused = errors.New("request ID already used")
)

type Reference struct {
	RequirementID string `json:"requirementId"`
	Version       int32  `json:"version"`
}

type Source struct {
	Reference
	Status string `json:"status"`
}

type Input struct {
	RequestID   string
	Analysis    string
	Limitations string
	Client      string
	Model       string
	Sharing     string
	Sources     []Reference
}

type Proposal struct {
	ID          string
	Analysis    string
	Limitations string
	Client      string
	Model       string
	Sharing     string
	CreatedAt   time.Time
	Sources     []Source
}

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Save(ctx context.Context, workspace, projectID string, in Input) (*Proposal, []project.FieldError, error) {
	fields := validate(&in)
	saved, err := s.save(ctx, workspace, projectID, in, len(fields) == 0)
	switch {
	case errors.Is(err, errSourceChanged):
		fields = append(fields, fieldError("CONFLICT", "validation.conflict", nil, "sources"))
	case errors.Is(err, errRequestReused):
		fields = append(fields, fieldError("CONFLICT", "validation.proposalRequestReused", nil, "requestId"))
	case err != nil:
		return nil, nil, err
	}
	return saved, fields, nil
}

func (s *Service) List(ctx context.Context, workspace, projectID string, first int, before *time.Time, beforeID string) ([]Proposal, error) {
	return s.list(ctx, workspace, projectID, first, before, beforeID)
}

func validate(in *Input) []project.FieldError {
	fields := []project.FieldError{}
	for _, field := range []struct {
		name  string
		value *string
		max   int
	}{
		{"requestId", &in.RequestID, 128},
		{"analysis", &in.Analysis, 10000},
		{"limitations", &in.Limitations, 4000},
		{"client", &in.Client, 200},
		{"model", &in.Model, 200},
	} {
		*field.value = strings.TrimSpace(*field.value)
		switch {
		case !utf8.ValidString(*field.value) || strings.ContainsRune(*field.value, '\x00'):
			fields = append(fields, fieldError("INVALID_UTF8", "validation.invalidEncoding", nil, field.name))
		case *field.value == "":
			fields = append(fields, fieldError("REQUIRED", "validation.required", nil, field.name))
		case utf8.RuneCountInString(*field.value) > field.max:
			fields = append(fields, fieldError("MAX_LENGTH", "validation.maxLength", map[string]int{"max": field.max}, field.name))
		}
	}
	if in.Sharing != "local" && in.Sharing != "cloud" {
		fields = append(fields, fieldError("INVALID_VALUE", "validation.proposalSharing", nil, "sharing"))
	}
	if len(in.Sources) < 1 || len(in.Sources) > 20 {
		return append(fields, fieldError("INVALID_COUNT", "validation.proposalSourceCount", nil, "sources"))
	}
	seen := make(map[string]bool, len(in.Sources))
	for i, source := range in.Sources {
		if source.Version < 1 || source.RequirementID == "" || len(source.RequirementID) > 128 ||
			!utf8.ValidString(source.RequirementID) || strings.ContainsRune(source.RequirementID, '\x00') || seen[source.RequirementID] {
			fields = append(fields, fieldError("INVALID_SOURCE", "validation.proposalSource", nil, "sources", strconv.Itoa(i)))
		}
		seen[source.RequirementID] = true
	}
	if len(fields) == 0 {
		in.Sources = slices.Clone(in.Sources)
		slices.SortFunc(in.Sources, func(a, b Reference) int {
			return strings.Compare(a.RequirementID, b.RequirementID)
		})
	}
	return fields
}

func fieldError(code, key string, params map[string]int, path ...string) project.FieldError {
	return project.FieldError{Path: append([]string{"input"}, path...), Code: code, MessageKey: key, Params: params}
}
