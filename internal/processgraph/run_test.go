package processgraph

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

type runGenerator struct {
	batches []Request
	calls   []Request
	split   []Request
	run     func(Request) (Suggestion, error)
}

func (g *runGenerator) GenerateProcessGraph(context.Context, Request) (Suggestion, error) {
	panic("a batch-capable adapter must use the single-call entry point")
}
func (g *runGenerator) PlanProcessGraph(Request) (BatchPlan, error) {
	return BatchPlan{Batches: g.batches}, nil
}
func (g *runGenerator) GenerateProcessGraphBatch(_ context.Context, req Request) (Suggestion, error) {
	g.calls = append(g.calls, req)
	return g.run(req)
}
func (g *runGenerator) SplitProcessGraphBatch(Request) (Request, Request, bool) {
	if len(g.split) != 2 {
		return Request{}, Request{}, false
	}
	return g.split[0], g.split[1], true
}

var sampleTarget = Target{Workspace: "team", Project: "product", Keyword: "delivery"}

func sampleRequest(ids ...string) Request {
	req := Request{Anchor: Anchor{Name: "delivery"}}
	for _, id := range ids {
		req.Sources = append(req.Sources, Source{ID: id, Text: "Delivery starts after approval.", Version: 3})
	}
	return req
}

func sampleRelation(req Request) Suggestion {
	return Suggestion{UsedSources: len(req.Sources), Nodes: []Node{
		{ID: "approval", Kind: "event", Label: "Approval"},
		{ID: "start", Kind: "action", Label: "Start delivery"},
	}, Edges: []SuggestedEdge{
		{From: "approval", To: "start", Kind: "triggers", Evidence: []SuggestedEvidence{{Source: 1, Quote: strings.TrimSpace(req.Sources[0].Text)}}},
	}}
}

func stableGraphSnapshot(context.Context, Target) (string, error) { return "original", nil }

func TestGraphRunSplitsRejectedBatchAndCountsSettledWork(t *testing.T) {
	all := sampleRequest("one", "two")
	first := sampleRequest("one")
	second := sampleRequest("two")
	g := &runGenerator{batches: []Request{all}, split: []Request{first, second}, run: func(req Request) (Suggestion, error) {
		if len(req.Sources) > 1 {
			return Suggestion{}, ErrBatchTooLarge
		}
		return sampleRelation(req), nil
	}}
	s := NewRunStore(g)
	start, err := s.Start(t.Context(), sampleTarget, all, "original", false)
	if err != nil || start.Status != "ready" || start.Run == nil || start.Run.Total != 1 || start.Run.Completed != 0 {
		t.Fatalf("unexpected plan: %+v, %v", start, err)
	}
	advance := func(revision int) RunResult {
		t.Helper()
		result, err := s.Advance(t.Context(), "team", "product", start.Run.ID, revision, stableGraphSnapshot)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	split := advance(start.Run.Revision)
	if split.Status != "ready" || split.Run.Total != 2 || split.Run.Completed != 0 || len(g.calls) != 1 {
		t.Fatalf("unexpected split result: %+v, calls=%d", split, len(g.calls))
	}
	if replay := advance(start.Run.Revision); !reflect.DeepEqual(replay, split) || len(g.calls) != 1 {
		t.Fatalf("replaying a request advanced the plan: %+v, calls=%d", replay, len(g.calls))
	}
	one := advance(split.Run.Revision)
	if one.Status != "ready" || one.Run.Total != 2 || one.Run.Completed != 1 || one.Draft != nil {
		t.Fatalf("unexpected first completed batch: %+v", one)
	}
	done := advance(one.Run.Revision)
	if done.Status != "suggested" || done.Run.Completed != 2 || done.Run.Failed != 0 ||
		done.Draft == nil || done.Draft.Partial || done.Draft.SourceCount != 2 ||
		len(done.Draft.Edges) != 1 || len(done.Draft.Edges[0].Evidence) != 2 {
		t.Fatalf("unexpected combined result: %+v, draft=%+v", done, done.Draft)
	}
}

func TestGraphRunPreservesLaterOffsetsAfterFailedBatch(t *testing.T) {
	first := sampleRequest("delivery")
	const prefix = "😀 Delivery starts after approval.\n"
	first.Sources[0].Text = prefix
	next := sampleRequest("delivery")
	next.Sources[0].Offset = utf8.RuneCountInString(prefix)
	missing := sampleRequest("middle")
	full := sampleRequest("delivery", "middle")
	full.Sources[0].Text = prefix + next.Sources[0].Text
	g := &runGenerator{batches: []Request{first, missing, next}, run: func(req Request) (Suggestion, error) {
		if req.Sources[0].ID == "middle" {
			return Suggestion{}, errors.New("provider failure")
		}
		return sampleRelation(req), nil
	}}
	s := NewRunStore(g)
	run, err := s.Start(t.Context(), sampleTarget, full, "original", false)
	if err != nil || run.Run == nil {
		t.Fatalf("start: %+v, %v", run, err)
	}
	for i := 0; i < 3; i++ {
		run, err = s.Advance(t.Context(), "team", "product", run.Run.ID, run.Run.Revision, stableGraphSnapshot)
		if err != nil {
			t.Fatal(err)
		}
		if run.Run.Completed != i+1 {
			t.Fatalf("unexpected progress: %+v", run.Run)
		}
	}
	if run.Status != "suggested" || run.Run.Failed != 1 || run.Draft == nil || !run.Draft.Partial ||
		run.Draft.SourceCount != 1 || len(run.Draft.Edges) != 1 || len(run.Draft.Edges[0].Evidence) != 2 {
		t.Fatalf("unexpected result after failure: %+v, draft=%+v", run, run.Draft)
	}
	later := run.Draft.Edges[0].Evidence[1]
	if later.Start != next.Sources[0].Offset || string([]rune(full.Sources[0].Text)[later.Start:later.End]) != later.Quote {
		t.Fatalf("later batch did not cite the original rune range: %+v", later)
	}
}

func TestGraphRunChecksScopeAndRejectsSourceChangesDuringInference(t *testing.T) {
	req := sampleRequest("source")
	key := "original"
	g := &runGenerator{batches: []Request{req}, run: func(req Request) (Suggestion, error) {
		key = "edited"
		return sampleRelation(req), nil
	}}
	s := NewRunStore(g)
	run, err := s.Start(t.Context(), sampleTarget, req, key, false)
	if err != nil || run.Run == nil {
		t.Fatalf("start: %+v, %v", run, err)
	}
	check := func(_ context.Context, target Target) (string, error) {
		if target != sampleTarget {
			t.Fatalf("wrong captured scope: %+v", target)
		}
		return key, nil
	}
	wrong, err := s.Advance(t.Context(), "other", "product", run.Run.ID, run.Run.Revision, check)
	if err != nil || wrong.Status != "not_found" || len(g.calls) != 0 {
		t.Fatalf("cross-workspace request accessed the run: %+v, %v", wrong, err)
	}
	stale, err := s.Advance(t.Context(), "team", "product", run.Run.ID, run.Run.Revision, check)
	if err != nil || stale.Status != "stale" || stale.Draft != nil || len(g.calls) != 1 {
		t.Fatalf("edited source yielded a draft: %+v, %v", stale, err)
	}
}

func TestMergeDraftKeepsDistinctBranchConditions(t *testing.T) {
	var combined Draft
	for _, label := range []string{"Code A", "Code a"} {
		mergeDraft(&combined, Draft{
			Nodes: []Node{
				{ID: "start", Kind: "event", Label: "Read code"},
				{ID: "finish", Kind: "action", Label: "Record result"},
			},
			Edges: []Edge{{
				From: "start", To: "finish", Kind: "triggers", Label: label,
				Evidence: []Evidence{{RequirementID: "source", Version: 1, Quote: label}},
			}},
		})
	}
	if len(combined.Nodes) != 2 || len(combined.Edges) != 2 ||
		combined.Edges[0].Label != "Code A" || combined.Edges[1].Label != "Code a" {
		t.Fatalf("branch alternatives collapsed: %+v", combined)
	}
}
