package processgraph

import (
	"context"
	"slices"
	"testing"
)

type stubGenerator func(context.Context, Request) (Suggestion, error)

func (f stubGenerator) GenerateProcessGraph(ctx context.Context, req Request) (Suggestion, error) {
	return f(ctx, req)
}

func TestBuildGroundsMixedRequirementFormats(t *testing.T) {
	req := Request{
		Anchor: Anchor{Name: "Заявка"},
		Sources: []Source{
			{ID: "story", Version: 1, Text: "Как оператор я хочу направить заявку на проверку."},
			{ID: "rule", Version: 2, Text: "Заявку вправе отклонить только руководитель."},
			{ID: "acceptance", Version: 3, Text: "Если заявку отклонили, статус меняется на «Отклонена»."},
			{ID: "scenario", Version: 4, Text: "Открыть заявку. Затем проверить реквизиты."},
			{ID: "statement", Version: 5, Text: "Команда «Направить заявку» запускает действие «Проверить реквизиты»."},
			{ID: "topic", Version: 6, Text: "Заявка"},
			{ID: "skipped", Version: 7, Text: "Заявка появляется в архиве."},
		},
	}
	rels := []SuggestedEdge{
		{From: "operator", To: "submit", Kind: "needs", Evidence: []SuggestedEvidence{{Source: 1, Quote: req.Sources[0].Text}}},
		{From: "supervisor", To: "reject", Kind: "permits", Label: "Только руководитель", Evidence: []SuggestedEvidence{{Source: 2, Quote: req.Sources[1].Text}}},
		{From: "reject", To: "rejected", Kind: "leads_to", Evidence: []SuggestedEvidence{{Source: 3, Quote: req.Sources[2].Text}}},
		{From: "open", To: "check", Kind: "follows", Evidence: []SuggestedEvidence{{Source: 4, Quote: req.Sources[3].Text}}},
		{From: "submit", To: "check", Kind: "triggers", Evidence: []SuggestedEvidence{{Source: 5, Quote: req.Sources[4].Text}}},
		{From: "check", To: "irrelevant", Kind: "follows", Evidence: []SuggestedEvidence{{Source: 6, Quote: req.Sources[5].Text}}},
		{From: "submit", To: "unsupported", Kind: "leads_to", Evidence: []SuggestedEvidence{{Source: 3, Quote: "Only the invented text appears."}}},
		{From: "check", To: "unsupported", Kind: "leads_to", Evidence: []SuggestedEvidence{{Source: 7, Quote: req.Sources[6].Text}}},
	}
	nodes := []Node{
		{ID: "operator", Kind: "actor", Label: "Оператор"},
		{ID: "supervisor", Kind: "actor", Label: "Руководитель"},
		{ID: "submit", Kind: "action", Label: "Направить заявку"},
		{ID: "check", Kind: "action", Label: "Проверить реквизиты"},
		{ID: "reject", Kind: "action", Label: "Отклонить заявку"},
		{ID: "rejected", Kind: "state", Label: "Отклонена"},
		{ID: "open", Kind: "action", Label: "Открыть заявку"},
		{ID: "irrelevant", Kind: "object", Label: "Только совпадение"},
		{ID: "unsupported", Kind: "object", Label: "Без источника"},
	}
	got, err := Build(t.Context(), stubGenerator(func(_ context.Context, r Request) (Suggestion, error) {
		if r.Anchor != req.Anchor || !slices.Equal(r.Sources, req.Sources) {
			t.Fatalf("generator did not receive the original request: %+v", r)
		}
		return Suggestion{Nodes: nodes, Edges: rels, UsedSources: 6}, nil
	}), req)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Partial || got.SourceCount != 6 || len(got.Nodes) != 7 || len(got.Edges) != 5 {
		t.Fatalf("expected five grounded edges and partial coverage, got %+v", got)
	}
	for i, edge := range got.Edges {
		src := req.Sources[i]
		ev := edge.Evidence[0]
		if ev.RequirementID != src.ID || ev.Version != src.Version ||
			string([]rune(src.Text)[ev.Start:ev.End]) != ev.Quote {
			t.Fatalf("incorrect source range or version: %+v", ev)
		}
	}
}

func TestBuildKeepsBranchesAndDropsAmbiguousNodeIDs(t *testing.T) {
	text := "После проверки: если ошибок нет — принять. Если найдены ошибки — вернуть."
	req := Request{Anchor: Anchor{Name: "проверка"}, Sources: []Source{{ID: "branch", Version: 1, Text: text}}}
	got, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1, Nodes: []Node{
			{ID: "check", Kind: "action", Label: "Проверка"},
			{ID: "approve", Kind: "action", Label: "Принять"},
			{ID: "return", Kind: "action", Label: "Вернуть"},
			{ID: "duplicate", Kind: "state", Label: "Одно"},
			{ID: "duplicate", Kind: "state", Label: "Другое"},
		}, Edges: []SuggestedEdge{
			{From: "check", To: "approve", Kind: "leads_to", Label: "Ошибок нет", Evidence: []SuggestedEvidence{{Source: 1, Quote: text}}},
			{From: "check", To: "return", Kind: "leads_to", Label: "Найдены ошибки", Evidence: []SuggestedEvidence{{Source: 1, Quote: text}}},
			{From: "approve", To: "duplicate", Kind: "leads_to", Evidence: []SuggestedEvidence{{Source: 1, Quote: text}}},
			{From: "check", To: "approve", Kind: "leads_to", Label: "Сумма верна", Evidence: []SuggestedEvidence{{Source: 1, Quote: text}}},
		}}, nil
	}), req)
	if err != nil || got.Partial || len(got.Nodes) != 3 || len(got.Edges) != 2 || got.Edges[1].Label != "Найдены ошибки" {
		t.Fatalf("unexpected branching result: %+v, %v", got, err)
	}
}

func TestBuildDropsRelationshipsWithIncompatibleEndpointKinds(t *testing.T) {
	text := "An operator sends the request. This launches approval."
	got, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		evidence := []SuggestedEvidence{{Source: 1, Quote: text}}
		return Suggestion{
			UsedSources: 1,
			Nodes: []Node{
				{ID: "actor", Kind: "actor", Label: "Operator"},
				{ID: "submit", Kind: "action", Label: "Send request"},
				{ID: "approve", Kind: "action", Label: "Approval"},
			},
			Edges: []SuggestedEdge{
				{From: "actor", To: "submit", Kind: "performs", Evidence: evidence},
				{From: "actor", To: "submit", Kind: "follows", Evidence: evidence},
				{From: "submit", To: "approve", Kind: "triggers", Evidence: evidence},
				{From: "submit", To: "actor", Kind: "triggers", Evidence: evidence},
			},
		}, nil
	}), Request{Anchor: Anchor{Name: "request"}, Sources: []Source{{ID: "rule", Version: 1, Text: text}}})
	if err != nil || len(got.Edges) != 2 || got.Edges[0].Kind != "performs" || got.Edges[1].Kind != "triggers" {
		t.Fatalf("invalid endpoint kinds were not discarded: %+v, %v", got, err)
	}
}

func TestBuildKeepsTheSelectedOccurrenceOfRepeatedEvidence(t *testing.T) {
	const quote = "Проверить заявку. При ошибке вернуть заявку."
	prefix := "😀 " + quote + "\n"
	start := len([]rune(prefix))
	text := prefix + quote
	got, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1, Nodes: []Node{
			{ID: "check", Kind: "action", Label: "Проверить"},
			{ID: "return", Kind: "action", Label: "Вернуть"},
		}, Edges: []SuggestedEdge{
			{From: "check", To: "return", Kind: "leads_to", Evidence: []SuggestedEvidence{{Source: 1, Quote: quote, Start: &start}}},
		}}, nil
	}), Request{Anchor: Anchor{Name: "заявка"}, Sources: []Source{{ID: "one", Version: 2, Text: text}}})
	if err != nil || len(got.Edges) != 1 || got.Edges[0].Evidence[0].Start != start ||
		got.Edges[0].Evidence[0].End != len([]rune(text)) {
		t.Fatalf("expected the selected second occurrence: %+v, %v", got, err)
	}
}

func TestBuildUsesLiteralRelationsWhenGenerationHasNoSupportedEdges(t *testing.T) {
	req := Request{Anchor: Anchor{Name: "заявка"}, Sources: []Source{
		{ID: "rule", Version: 2, Text: "😀 Правило. Если реквизиты верны, заявка получает статус «Принята». Если есть ошибки, то заявку возвращают."},
		{ID: "criterion", Version: 3, Text: "- If the request is incomplete, then return it for review."},
		{ID: "rule-permissions", Version: 1, Text: "Заказчик не может создавать заявки. Оператору запрещено удалять заявки."},
		{ID: "topic", Version: 1, Text: "Заявка упоминается в отчёте. К ней добавлено поле."},
	}}
	got, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 4}, nil
	}), req)
	if err != nil || !got.Partial || got.SourceCount != 4 || len(got.Edges) != 5 {
		t.Fatalf("expected five explicitly supported relations: %+v, %v", got, err)
	}
	for i, edge := range got.Edges {
		kind := "leads_to"
		if i >= 3 {
			kind = "prohibits"
		}
		if edge.Kind != kind || len(edge.Evidence) != 1 {
			t.Fatalf("unexpected relation: %+v", edge)
		}
	}
	first := got.Edges[0].Evidence[0]
	if first.RequirementID != "rule" || first.Version != 2 ||
		string([]rune(req.Sources[0].Text)[first.Start:first.End]) != first.Quote ||
		first.Quote != "Если реквизиты верны, заявка получает статус «Принята»" {
		t.Fatalf("evidence should be the exact conditional clause: %+v", first)
	}
}

func TestBuildRecoversExplicitNeedsFromUserStories(t *testing.T) {
	sources := []Source{
		{ID: "ru-story", Version: 2, Text: "Я, как сотрудник ГП, хочу, чтобы инспекции отображались в реестре, для того чтобы контролировать сроки."},
		{ID: "ru-need", Version: 3, Text: "Как представитель ГП я должен иметь возможность выгружать реестр, чтобы подготовить отчёт."},
		{ID: "en-story", Version: 1, Text: "As a contractor, I want to see the inspection status so that I can plan the work."},
		{ID: "mention", Version: 1, Text: "Инспекции и сотрудники ГП перечислены в отчёте."},
	}
	req := Request{Anchor: Anchor{Name: "Генподрядчик"}, Sources: sources}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: len(sources)}, nil
	}), req)
	if err != nil || !draft.Partial || len(draft.Edges) != 3 || len(draft.Nodes) != 6 {
		t.Fatalf("expected only the explicitly stated role needs: %+v, %v", draft, err)
	}
	for i, edge := range draft.Edges {
		source := sources[i]
		evidence := edge.Evidence[0]
		if edge.Kind != "needs" || evidence.RequirementID != source.ID || evidence.Version != source.Version ||
			string([]rune(source.Text)[evidence.Start:evidence.End]) != evidence.Quote {
			t.Fatalf("unexpected grounded relationship: %+v", edge)
		}
	}
	labels := []string{
		"сотрудник ГП", "инспекции отображались в реестре",
		"представитель ГП", "выгружать реестр",
		"contractor", "see the inspection status",
	}
	for i, node := range draft.Nodes {
		if node.Label != labels[i] {
			t.Fatalf("node %d: expected %q, got %q", i, labels[i], node.Label)
		}
	}
}
