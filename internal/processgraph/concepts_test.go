package processgraph

import (
	"context"
	"testing"

	"github.com/VolkovDani/project-creator/internal/glossary"
)

func projectRoleGlossary() []glossary.Term {
	return []glossary.Term{
		{ID: "general", Name: "Генподрядчик", Version: 1, Variants: []glossary.Variant{
			{Text: "представитель ген. подрядной организации"},
			{Text: "Генподрядчику"},
		}},
		{ID: "contractor", Name: "Подрядчик", Version: 1, Variants: []glossary.Variant{
			{Text: "представитель подрядной организации"},
			{Text: "подрядной организации"},
		}},
	}
}

func TestGraphKeepsRoleEvidenceTiedToSelectedConcept(t *testing.T) {
	const general = "Генподрядчик проверяет реестр."
	const separate = "Представитель подрядной организации копирует заявку."
	const connected = "Представитель подрядной организации отправляет отчёт Генподрядчику."
	req := Request{
		Anchor: Anchor{Name: "Генподрядчик", ConceptID: "general"}, Glossary: projectRoleGlossary(),
		Sources: []Source{
			{ID: "mixed", Version: 1, Text: general + " " + separate},
			{ID: "connected", Version: 2, Text: connected},
		},
	}
	makeEdge := func(from, to string, source int, quote string) SuggestedEdge {
		return SuggestedEdge{From: from, To: to, Kind: "performs", Evidence: []SuggestedEvidence{{Source: source, Quote: quote}}}
	}
	got, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 2, Nodes: []Node{
			{ID: "general", Kind: "actor", Label: "Генподрядчик"},
			{ID: "contractor", Kind: "actor", Label: "Представитель подрядной организации"},
			{ID: "merged", Kind: "actor", Label: "Генподрядчик и Подрядчик"},
			{ID: "check", Kind: "action", Label: "Проверить реестр"},
			{ID: "copy", Kind: "action", Label: "Копировать"},
			{ID: "send", Kind: "action", Label: "Отправить отчёт"},
		}, Edges: []SuggestedEdge{
			makeEdge("general", "check", 1, general),
			makeEdge("contractor", "copy", 1, separate),
			makeEdge("contractor", "check", 1, general),
			{From: "contractor", To: "copy", Kind: "performs", Evidence: []SuggestedEvidence{
				{Source: 1, Quote: general}, {Source: 1, Quote: separate},
			}},
			makeEdge("merged", "send", 2, connected),
			makeEdge("contractor", "send", 2, connected),
		}}, nil
	}), req)
	if err != nil || len(got.Edges) != 2 || got.Edges[0].To != "check" || got.Edges[0].From != "general" ||
		got.Edges[1].To != "send" || got.Edges[1].From != "contractor" {
		t.Fatalf("expected only the grounded selected and cross-role links: %+v, %v", got, err)
	}
}

func TestGraphChecksFullGlossaryMentionWhenCitationsSplitAnAlias(t *testing.T) {
	const head = "Представитель ген."
	const tail = "подрядной организации оформляет заявку."
	req := Request{
		Anchor: Anchor{Name: "Генподрядчик", ConceptID: "general"}, Glossary: projectRoleGlossary(),
		Sources: []Source{{ID: "source", Version: 1, Text: head + " " + tail}},
	}
	got, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1, Nodes: []Node{
			{ID: "general", Kind: "actor", Label: "Генподрядчик"},
			{ID: "wrong", Kind: "actor", Label: "Представитель подрядной организации"},
			{ID: "file", Kind: "action", Label: "Оформляет заявку"},
		}, Edges: []SuggestedEdge{
			{From: "general", To: "file", Kind: "performs", Evidence: []SuggestedEvidence{{Source: 1, Quote: head}, {Source: 1, Quote: tail}}},
			{From: "wrong", To: "file", Kind: "performs", Evidence: []SuggestedEvidence{{Source: 1, Quote: head}, {Source: 1, Quote: tail}}},
		}}, nil
	}), req)
	if err != nil || len(got.Edges) != 1 || got.Edges[0].From != "general" {
		t.Fatalf("the qualified role should be grounded across the two citations: %+v, %v", got, err)
	}
}

func TestPlainGraphGeneratorReceivesMatchedVocabularyInBatch(t *testing.T) {
	req := sampleRequest("one")
	req.Glossary, req.Anchor.ConceptID = projectRoleGlossary(), "general"
	store := NewRunStore(stubGenerator(func(_ context.Context, batch Request) (Suggestion, error) {
		if batch.Anchor.ConceptID != "general" || len(batch.Glossary) != len(req.Glossary) {
			t.Fatalf("batch lost glossary context: %+v", batch)
		}
		return sampleRelation(batch), nil
	}))
	run, err := store.Start(t.Context(), sampleTarget, req, "original", false)
	if err != nil || run.Run == nil {
		t.Fatalf("start: %+v, %v", run, err)
	}
	done, err := store.Advance(t.Context(), "team", "product", run.Run.ID, run.Run.Revision, stableGraphSnapshot)
	if err != nil || done.Status != "suggested" {
		t.Fatalf("unexpected batch result: %+v, %v", done, err)
	}
}
