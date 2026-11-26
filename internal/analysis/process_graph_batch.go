package analysis

import (
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

var _ processgraph.BatchGenerator = (*chatSummary)(nil)

func (s *chatSummary) PlanProcessGraph(req processgraph.Request) (processgraph.BatchPlan, error) {
	output, err := s.processGraphOutput(req.Mode)
	if err != nil {
		return processgraph.BatchPlan{}, err
	}
	matchGlossary := processGraphGlossaryMatches(req)
	return planProcessGraph(req, func(batch processgraph.Request) (bool, error) {
		_, err := s.prepareProcessGraphBatch(batch, matchGlossary, output)
		if errors.Is(err, processgraph.ErrBatchTooLarge) {
			return false, nil
		}
		return err == nil, err
	})
}

func planProcessGraph(req processgraph.Request, fits func(processgraph.Request) (bool, error)) (processgraph.BatchPlan, error) {
	var plan processgraph.BatchPlan
	minimum := 1
	sources := req.Sources
	if req.Mode == processgraph.RequirementMode {
		minimum = 2
		sources = slices.Clone(sources)
	}
	for len(sources) >= minimum {
		count, err := largestProcessGraphPrefix(len(sources), func(count int) (bool, error) {
			return fits(processGraphBatchRequest(req, sources[:count]))
		})
		if err != nil {
			return processgraph.BatchPlan{}, err
		}
		if count < minimum {
			plan.Partial = true
			if minimum == 2 && count == 1 {
				// An oversized next source must not prevent comparing the
				// fitting first source with a later one.
				nextFits, err := fits(processGraphBatchRequest(req, sources[1:2]))
				if err != nil {
					return processgraph.BatchPlan{}, err
				}
				if !nextFits {
					sources = slices.Delete(sources, 1, 2)
					continue
				}
			}
			sources = sources[1:]
			continue
		}
		plan.Batches = append(plan.Batches, processGraphBatchRequest(req, sources[:count]))
		if count == len(sources) {
			break
		}
		if req.Mode == processgraph.RequirementMode {
			// Retain one whole source so adjacent windows can share a node.
			plan.Partial = true
			count--
		}
		sources = sources[count:]
	}
	return plan, nil
}

// Grow the candidate around the maximum fitting prefix instead of sending all
// remaining requirements on every tokenizer probe.
func largestProcessGraphPrefix(total int, fits func(int) (bool, error)) (int, error) {
	best := 0
	for probe := 1; ; probe = min(probe*2, total) {
		ok, err := fits(probe)
		if err != nil {
			return 0, err
		}
		if ok {
			if probe == total {
				return probe, nil
			}
			best = probe
			continue
		}
		firstUnfit := sort.Search(probe-best-1, func(i int) bool {
			if err != nil {
				return true
			}
			var fitting bool
			fitting, err = fits(best + i + 1)
			return err != nil || !fitting
		})
		return best + firstUnfit, err
	}
}

func (s *chatSummary) GenerateProcessGraphBatch(ctx context.Context, req processgraph.Request) (processgraph.Suggestion, error) {
	return s.generateProcessGraphBatch(ctx, req, false)
}

func (s *chatSummary) generateProcessGraphBatch(ctx context.Context, req processgraph.Request, usePreflight bool) (processgraph.Suggestion, error) {
	output, err := s.processGraphOutput(req.Mode)
	if err != nil {
		return processgraph.Suggestion{}, err
	}
	matchGlossary := processGraphGlossaryMatches(req)
	var input processGraphInput
	if usePreflight {
		input, err = renderProcessGraph(req, matchGlossary, s.processGraphPrompt(req, output.edgeLimit), 0, false, nil)
	} else {
		input, err = s.prepareProcessGraphBatch(req, matchGlossary, output)
	}
	if err != nil {
		return processgraph.Suggestion{}, err
	}
	if err := s.verify(ctx); err != nil {
		return processgraph.Suggestion{}, err
	}
	content, err := s.complete(ctx, input.messages, output.schema)
	npuInputRejected := s.managedNPU && errors.Is(err, npuFailure("NPU_UNAVAILABLE"))
	if ctx.Err() == nil && (npuInputRejected || errors.Is(err, errChatOutputLimit)) {
		return processgraph.Suggestion{}, processgraph.ErrBatchTooLarge
	}
	if err != nil {
		return processgraph.Suggestion{}, err
	}
	return s.decodeGraph(ctx, req.Mode, input, content, output.edgeLimit)
}

func (s *chatSummary) prepareProcessGraphBatch(req processgraph.Request, matchGlossary processGraphGlossaryMatcher, output processGraphOutput) (processGraphInput, error) {
	prompt := s.processGraphPrompt(req, output.edgeLimit)
	input, err := prepareProcessGraph(req, matchGlossary, prompt, output.schemaBytes, s.managedNPU, 0, false)
	if errors.Is(err, ErrSummaryInputTooLong) || input.partial {
		return processGraphInput{}, processgraph.ErrBatchTooLarge
	}
	return input, err
}

func (s *chatSummary) SplitProcessGraphBatch(req processgraph.Request) (processgraph.Request, processgraph.Request, bool) {
	if len(req.Sources) < 2 {
		return processgraph.Request{}, processgraph.Request{}, false
	}
	middle := len(req.Sources) / 2
	right := middle
	if req.Mode == processgraph.RequirementMode {
		if len(req.Sources) < 3 {
			return processgraph.Request{}, processgraph.Request{}, false
		}
		middle = (len(req.Sources) + 1) / 2
		right = middle - 1
	}
	return processGraphBatchRequest(req, req.Sources[:middle]), processGraphBatchRequest(req, req.Sources[right:]), true
}

func processGraphBatchRequest(req processgraph.Request, sources []processgraph.Source) processgraph.Request {
	batch := req
	batch.Sources = slices.Clone(sources)
	if len(batch.OriginalSources) == 0 {
		batch.OriginalSources = req.Sources
	}
	return batch
}
