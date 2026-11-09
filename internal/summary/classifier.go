package summary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/VolkovDani/project-creator/contracts"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

// Classifier replaces only the preliminary model's texts. Pair source snapshots
// stay unchanged and the final LLM always receives full originals.
type Classifier struct {
	s       *Service
	next    analysis.Classifier
	profile string
}

func (s *Service) Classifier(next analysis.Classifier) analysis.Classifier {
	if next == nil {
		return next
	}
	sum := sha256.Sum256([]byte("summary-input-v2:" + s.Profile() + ":" + next.Profile()))
	return &Classifier{s: s, next: next, profile: hex.EncodeToString(sum[:])}
}
func (c *Classifier) Profile() string { return c.profile }
func (c *Classifier) Provenance() string {
	classifier := c.next.Profile()
	if p, ok := c.next.(interface{ Provenance() string }); ok {
		classifier = p.Provenance()
	}
	summary := "not_configured"
	if c.s.generator != nil {
		summary = c.s.generator.Provenance()
	}
	raw, _ := json.Marshal(map[string]string{"adapter": "summary-input-v2", "summary": summary, "classifier": classifier})
	return string(raw)
}
func (c *Classifier) Classify(ctx context.Context, p analysis.Pair) (analysis.Preclassification, error) {
	return analysis.Preclassification{}, errors.New("classification scope required")
}
func (c *Classifier) ClassifyScoped(ctx context.Context, w, project string, p analysis.Pair) (analysis.Preclassification, error) {
	if c.s.generator == nil {
		return analysis.Preclassification{}, analysis.ErrSummaryUnavailable
	}
	rows, e := c.s.ByIDs(ctx, w, project, []string{p.Left.ID, p.Right.ID})
	if e != nil {
		return analysis.Preclassification{}, analysis.ErrSummaryUnavailable
	}
	for i, source := range []*analysis.Source{&p.Left, &p.Right} {
		v := rows[i]
		if v.Status != "ready" || v.Version != source.Version {
			return analysis.Preclassification{}, analysis.ErrSummaryUnavailable
		}
		if !contracts.SummaryLayaEligible(v.Text) {
			return analysis.Preclassification{}, analysis.ErrClassificationInputTooLong
		}
		var text string
		// Verify the original snapshot before releasing the derived text to the model.
		if e = c.s.db.QueryRowContext(ctx, `SELECT text FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4`, w, project, source.ID, source.Version).Scan(&text); e != nil || text != source.Text {
			return analysis.Preclassification{}, analysis.ErrSummaryUnavailable
		}
		source.Text = v.Text
	}
	return c.next.Classify(ctx, p)
}
