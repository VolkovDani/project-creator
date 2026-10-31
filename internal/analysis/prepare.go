package analysis

import "context"

func (s *Service) prepare(ctx context.Context, workspace, project string, sources []Source, incomplete bool, findingPairs func() error) ([]Pair, string, bool, diagnostic) {
	pairs := []Pair{}
	d := diagnostic{MissingRequirementIDs: []string{}}
	if incomplete {
		d.ReasonCode = "REQUIREMENT_LIMIT"
		return pairs, "", true, d
	}
	if len(sources) < 2 {
		d.ReasonCode = "TOO_FEW_REQUIREMENTS"
		return pairs, "", true, d
	}
	if s.generator == nil || s.generatorProfile() == "" || s.retriever == nil || s.embeddingProfile == "" {
		return pairs, "NOT_CONFIGURED", true, d
	}
	states, err := s.retriever.Status(ctx, workspace, project, 100, 0)
	if err != nil {
		if ctx.Err() != nil {
			return pairs, "TIMEOUT", true, d
		}
		return pairs, "RETRIEVAL_FAILED", true, d
	}
	d.MissingRequirementIDs = missingIndexes(sources, states, s.embeddingProfile)
	if len(d.MissingRequirementIDs) > 0 {
		d.ReasonCode = "INDEX_INCOMPLETE"
		return pairs, "", true, d
	}
	if findingPairs != nil {
		if err = findingPairs(); err != nil {
			return pairs, "INTERRUPTED", true, d
		}
	}
	byID := map[string]Source{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if !validSource(source) {
			incomplete = true
			continue
		}
		hits, err := s.retriever.Search(ctx, workspace, project, source.Text, 6)
		if err != nil {
			if ctx.Err() != nil {
				return pairs, "TIMEOUT", true, d
			}
			return pairs, "RETRIEVAL_FAILED", true, d
		}
		for _, hit := range hits {
			other, exists := byID[hit.RequirementID]
			if !exists || other.ID == source.ID || other.Version != hit.Version {
				continue
			}
			if !validSource(other) {
				incomplete = true
				continue
			}
			left, right := source, other
			if left.ID > right.ID {
				left, right = right, left
			}
			key := left.ID + "\x00" + right.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(pairs) >= 20 {
				incomplete = true
				continue
			}
			pairs = append(pairs, Pair{Left: left, Right: right})
		}
	}
	return pairs, "", incomplete, d
}
