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
	var plan processgraph.BatchPlan
	for passages := processGraphPassages(req); len(passages) > 0; {
		var prepareErr error
		tooLarge := func(index int) bool {
			batch := processGraphBatchRequest(req, passages[:index+1])
			_, err := s.prepareProcessGraphBatch(batch, output)
			if err != nil && !errors.Is(err, processgraph.ErrBatchTooLarge) {
				prepareErr = err
			}
			return err != nil
		}
		// A fitting request needs exactly one completion. Otherwise find an
		// advancing prefix at a passage boundary within the rendered budget.
		count := len(passages)
		if tooLarge(count - 1) {
			count = sort.Search(count, tooLarge)
		}
		if prepareErr != nil {
			return processgraph.BatchPlan{}, prepareErr
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

func (s *chatSummary) GenerateProcessGraphBatch(ctx context.Context, req processgraph.Request) (processgraph.Suggestion, error) {
	output, err := s.processGraphOutput()
	if err != nil {
		return processgraph.Suggestion{}, err
	}
	input, err := s.prepareProcessGraphBatch(req, output)
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

func (s *chatSummary) prepareProcessGraphBatch(req processgraph.Request, output processGraphOutput) (processGraphInput, error) {
	prompt := s.processGraphPrompt(req, output.edgeLimit)
	input, err := prepareProcessGraph(req, prompt, output.schemaBytes, s.managedNPU, 0, false)
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
	batch := processgraph.Request{Anchor: req.Anchor}
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
