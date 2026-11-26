package analysis

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

var _ processgraph.BatchGenerator = (*chatSummary)(nil)

func (s *chatSummary) PlanProcessGraph(req processgraph.Request) (processgraph.BatchPlan, error) {
	output, err := s.processGraphOutput()
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
	for passages := processGraphPassages(req); len(passages) > 0; {
		count, err := largestProcessGraphPrefix(len(passages), func(count int) (bool, error) {
			return fits(processGraphBatchRequest(req, passages[:count]))
		})
		if err != nil {
			return processgraph.BatchPlan{}, err
		}
		if count == 0 {
			plan.Partial = true
			passages = passages[1:]
			continue
		}
		plan.Batches = append(plan.Batches, processGraphBatchRequest(req, passages[:count]))
		passages = passages[count:]
	}
	return plan, nil
}

// Grow the candidate around the maximum fitting prefix instead of sending all
// remaining passages on every tokenizer probe.
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
	output, err := s.processGraphOutput()
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
	if s.managedNPU && errors.Is(err, npuFailure("NPU_UNAVAILABLE")) && ctx.Err() == nil {
		return processgraph.Suggestion{}, processgraph.ErrBatchTooLarge
	}
	if err != nil {
		return processgraph.Suggestion{}, err
	}
	return s.decodeProcessGraph(ctx, input, content, output.edgeLimit)
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
	passages := processGraphPassages(req)
	if len(passages) < 2 {
		return processgraph.Request{}, processgraph.Request{}, false
	}
	middle := len(passages) / 2
	return processGraphBatchRequest(req, passages[:middle]), processGraphBatchRequest(req, passages[middle:]), true
}

type processGraphPassage struct {
	source     int
	start, end int // Byte offsets in the request source fragment.
	offset     int // Rune offset in the original requirement.
}

func processGraphPassages(req processgraph.Request) []processGraphPassage {
	var passages []processGraphPassage
	for index, source := range req.Sources {
		start, offset := 0, source.Offset
		for _, passage := range explanationPassages(source.Text)[1:] {
			end := start + len(passage)
			if strings.TrimSpace(passage) != "" {
				passages = append(passages, processGraphPassage{source: index, start: start, end: end, offset: offset})
			}
			start = end
			offset += utf8.RuneCountInString(passage)
		}
	}
	return passages
}

func processGraphBatchRequest(req processgraph.Request, passages []processGraphPassage) processgraph.Request {
	batch := req
	batch.Sources = nil
	if len(batch.OriginalSources) == 0 {
		batch.OriginalSources = req.Sources
	}
	for len(passages) > 0 {
		first, end := passages[0], passages[0].end
		count := 1
		for count < len(passages) && passages[count].source == first.source && passages[count].start == end {
			end = passages[count].end
			count++
		}
		source := req.Sources[first.source]
		source.Text = source.Text[first.start:end]
		source.Offset = first.offset
		batch.Sources = append(batch.Sources, source)
		passages = passages[count:]
	}
	return batch
}
