package processgraph

import (
	"context"
	"strings"
	"testing"
)

func TestGraphRunAddsLiteralStoryChainWhenAnotherModelEdgeSurvives(t *testing.T) {
	req := Request{Anchor: Anchor{Name: "проверяющий"}, Sources: []Source{
		{ID: "permission", Version: 1, Text: "Руководитель вправе открыть карточку."},
		{ID: "story", Version: 4, Text: "Я, как проверяющий, хочу, чтобы при нажатии кнопки «Готово», карточка переходила в архив, для того, чтобы быстро закрыть работу."},
	}}
	gen := &runGenerator{batches: []Request{{Anchor: req.Anchor, Sources: req.Sources[:1]}}, run: func(req Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1, Nodes: []Node{
			{ID: "manager", Kind: "actor", Label: "Руководитель"},
			{ID: "open", Kind: "action", Label: "открыть карточку"},
		}, Edges: []SuggestedEdge{{From: "manager", To: "open", Kind: "permits", Evidence: []SuggestedEvidence{{Source: 1, Quote: req.Sources[0].Text}}}}}, nil
	}}
	store := NewRunStore(gen)
	started, err := store.Start(t.Context(), sampleTarget, req, "original", false)
	if err != nil || started.Run == nil {
		t.Fatalf("start: %+v, %v", started, err)
	}
	done, err := store.Advance(t.Context(), sampleTarget.Workspace, sampleTarget.Project, started.Run.ID, started.Run.Revision, stableGraphSnapshot)
	if err != nil || done.Status != "suggested" || done.Draft == nil || len(done.Draft.Edges) != 3 || !done.Draft.Partial || done.Draft.SourceCount != 2 {
		t.Fatalf("expected the model edge plus a sourced literal chain: %+v, draft=%+v, err=%v", done, done.Draft, err)
	}
	assertLiteralStoryChain(t, *done.Draft, req.Sources[1], "проверяющий", "нажатии кнопки «Готово»", "карточка переходила в архив", "быстро закрыть работу")
}

func TestBuildKeepsLiteralStoryChainsWithinBudget(t *testing.T) {
	req := Request{Anchor: Anchor{Name: "clerk"}}
	for i := range MaxEdges {
		req.Sources = append(req.Sources, Source{ID: string(rune('a' + i)), Version: 1, Text: "As a clerk, I want to open list " + string(rune('A'+i)) + " to check its contents."})
	}
	story := Source{ID: "last", Version: 7, Text: "As a clerk, I want that when I click Archive, the item moves to the archive so that I can finish the report."}
	req.Sources = append(req.Sources, story)
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: len(req.Sources)}, nil
	}), req)
	if err != nil || len(draft.Edges) != MaxEdges || !draft.Partial || draft.SourceCount != len(req.Sources) {
		t.Fatalf("expected bounded literal links across the entire scope: %+v, err=%v", draft, err)
	}
	assertLiteralStoryChain(t, draft, story, "clerk", "I click Archive", "the item moves to the archive", "finish the report")
}

func TestBuildDoesNotInferPurposeOrJoinIndependentClauses(t *testing.T) {
	source := Source{ID: "story", Version: 2, Text: "Я, как проверяющий, хочу открыть список, чтобы завершить отчёт. После подписания записки, запись блокируется."}
	req := Request{Anchor: Anchor{Name: "проверяющий"}, Sources: []Source{source}}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1}, nil
	}), req)
	if err != nil || len(draft.Edges) != 2 {
		t.Fatalf("expected independent role need and transition: %+v, err=%v", draft, err)
	}
	nodes := literalTestNodes(draft)
	for _, edge := range draft.Edges {
		switch edge.Kind {
		case "needs":
			if nodes[edge.From].Label != "проверяющий" || nodes[edge.To].Label != "открыть список" {
				t.Fatalf("the independent story must keep its original request: %+v", draft)
			}
		case "leads_to":
			if nodes[edge.From].Label != "подписания записки" || nodes[edge.To].Label != "запись блокируется" {
				t.Fatalf("the independent transition must keep its explicit trigger: %+v", draft)
			}
		default:
			t.Fatalf("unexpected relation: %+v", edge)
		}
	}
}

func TestBuildKeepsExplicitStatusResultWithoutAComma(t *testing.T) {
	source := Source{ID: "story", Version: 3, Text: "Я, как контролёр, хочу, чтобы при копировании записи статус новой записи автоматически становился «Черновик» для того чтобы подготовить отчёт."}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1}, nil
	}), Request{Anchor: Anchor{Name: "контролёр"}, Sources: []Source{source}})
	if err != nil || len(draft.Edges) != 2 {
		t.Fatalf("expected an explicit story transition: %+v, err=%v", draft, err)
	}
	assertLiteralStoryChain(t, draft, source, "контролёр", "копировании записи", "статус новой записи автоматически становился «Черновик»", "подготовить отчёт")
}

func TestBuildKeepsTheCompleteOutcomeSubject(t *testing.T) {
	const outcome = "регистрационный номер становится недоступен для редактирования"
	for _, action := range []string{"направления заявки", "направления"} {
		t.Run(action, func(t *testing.T) {
			source := Source{ID: "rule", Version: 1, Text: "После " + action + " " + outcome + "."}
			draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
				return Suggestion{UsedSources: 1}, nil
			}), Request{Anchor: Anchor{Name: "номер"}, Sources: []Source{source}})
			if err != nil || len(draft.Edges) != 1 {
				t.Fatalf("expected one explicit transition: %+v, err=%v", draft, err)
			}
			nodes := literalTestNodes(draft)
			edge := draft.Edges[0]
			if nodes[edge.From].Label != action || nodes[edge.To].Label != outcome {
				t.Fatalf("expected the complete result subject: %+v", draft.Nodes)
			}
		})
	}
}

func TestBuildRejectsPurposeAsStandaloneOutcome(t *testing.T) {
	source := Source{ID: "rule", Version: 1, Text: "При нажатии кнопки статус меняется, для того чтобы сверить отчёт. При открытии отчёта, чтобы сверить данные."}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1}, nil
	}), Request{Anchor: Anchor{Name: "кнопка"}, Sources: []Source{source}})
	if err != nil || len(draft.Edges) != 1 {
		t.Fatalf("expected only the explicit status change: %+v, err=%v", draft, err)
	}
	nodes := literalTestNodes(draft)
	edge := draft.Edges[0]
	if edge.Kind != "leads_to" || nodes[edge.From].Label != "нажатии кнопки" || nodes[edge.To].Label != "статус меняется" {
		t.Fatalf("the explicit result must be kept apart from the purpose: %+v", draft)
	}
}

func TestBuildLeavesDependentStoryClausesOutOfResults(t *testing.T) {
	for _, text := range []string{
		"Я, как оператор, хочу, чтобы при создании карточки я мог указать код, выбрав его из списка.",
		"Я, как оператор, хочу, чтобы при создании карточки можно было указать элемент, к которому относится запись.",
		"Я, как оператор, хочу, чтобы при создании карточки, выбрав код, форма открывалась в новой вкладке.",
		"Я, как оператор, хочу, чтобы при создании карточки я мог указать код, форма открывалась в новой вкладке.",
	} {
		t.Run(text, func(t *testing.T) {
			req := Request{Anchor: Anchor{Name: "оператор"}, Sources: []Source{{ID: "story", Version: 1, Text: text}}}
			draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
				return Suggestion{UsedSources: 1}, nil
			}), req)
			if err != nil || len(draft.Edges) != 1 || draft.Edges[0].Kind != "needs" {
				t.Fatalf("expected the requested behavior without an inferred result: %+v, err=%v", draft, err)
			}
		})
	}
}

func TestBuildAvoidsDuplicateLiteralForModelEdgeAndRetainsAnotherClause(t *testing.T) {
	const first = "Если реквизиты проверили, статус дела меняется на «Зарегистрировано»"
	source := Source{ID: "rule", Version: 3, Text: first + ". Если реквизиты не проверили, статус дела меняется на «Зарегистрировано»."}
	req := Request{Anchor: Anchor{Name: "заявление"}, Sources: []Source{source}}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1, Nodes: []Node{
			{ID: "3", Kind: "action", Label: "Проверить реквизиты"},
			{ID: "outcome", Kind: "state", Label: "Зарегистрировано"},
		}, Edges: []SuggestedEdge{{From: "3", To: "outcome", Kind: "leads_to", Evidence: []SuggestedEvidence{{Source: 1, Quote: source.Text}}}}}, nil
	}), req)
	if err != nil || len(draft.Edges) != 2 || draft.Edges[0].Evidence[0].Quote != source.Text ||
		!strings.HasPrefix(draft.Edges[1].Evidence[0].Quote, "Если реквизиты не проверили") {
		t.Fatalf("expected each outcome once: %+v, err=%v", draft, err)
	}
	nodes := literalTestNodes(draft)
	if len(nodes) != len(draft.Nodes) || nodes["3"].Label != "Проверить реквизиты" {
		t.Fatalf("new nodes collided with existing IDs: %+v", draft.Nodes)
	}
}

func TestBuildDoesNotDeduplicateDifferentExplicitConditions(t *testing.T) {
	source := Source{ID: "rule", Version: 1, Text: "Если реквизиты проверили утром, отчёт сохраняется. Если реквизиты проверили вечером, отчёт сохраняется."}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1, Nodes: []Node{
			{ID: "a", Kind: "action", Label: "Проверить реквизиты"},
			{ID: "b", Kind: "state", Label: "отчёт сохраняется"},
		}, Edges: []SuggestedEdge{{From: "a", To: "b", Kind: "leads_to", Label: "утром", Evidence: []SuggestedEvidence{{Source: 1, Quote: source.Text}}}}}, nil
	}), Request{Anchor: Anchor{Name: "реквизиты"}, Sources: []Source{source}})
	if err != nil || len(draft.Edges) != 2 || draft.Edges[0].Label != "утром" || !strings.HasPrefix(draft.Edges[1].Evidence[0].Quote, "Если реквизиты проверили вечером") {
		t.Fatalf("expected distinct conditions for the shared result: %+v, err=%v", draft, err)
	}
}

func assertLiteralStoryChain(t *testing.T, draft Draft, source Source, role, action, outcome, purpose string) {
	t.Helper()
	nodes := literalTestNodes(draft)
	var need, result string
	for _, edge := range draft.Edges {
		from, to := nodes[edge.From], nodes[edge.To]
		if strings.Contains(from.Label, purpose) || strings.Contains(to.Label, purpose) {
			t.Fatalf("business purpose became a node: %+v → %+v", from, to)
		}
		switch {
		case edge.Kind == "needs" && from.Label == role && to.Label == action:
			need = to.ID
		case edge.Kind == "leads_to" && from.Label == action && to.Label == outcome:
			result = from.ID
		default:
			continue
		}
		if len(edge.Evidence) != 1 {
			t.Fatalf("expected one exact supporting citation: %+v", edge)
		}
		ev := edge.Evidence[0]
		if ev.RequirementID != source.ID || ev.Version != source.Version || string([]rune(source.Text)[ev.Start:ev.End]) != ev.Quote {
			t.Fatalf("unexpected evidence: %+v", ev)
		}
	}
	if need == "" || need != result {
		t.Fatalf("expected a shared action between role and result: %+v", draft)
	}
}

func literalTestNodes(draft Draft) map[string]Node {
	nodes := make(map[string]Node, len(draft.Nodes))
	for _, node := range draft.Nodes {
		nodes[node.ID] = node
	}
	return nodes
}

func TestBuildConnectsAcceptanceTransitionToTheStoryRole(t *testing.T) {
	const story = "Я, как оператор, должен иметь возможность указать регистрационный номер записи, чтобы найти запись"
	const criterion = "После направления записи регистрационный номер становится недоступен для редактирования"
	source := Source{ID: "story", Version: 4, Text: story + ".\n\n" + criterion + "."}
	req := Request{Anchor: Anchor{Name: "оператор"}, Sources: []Source{source}}
	draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
		return Suggestion{UsedSources: 1}, nil
	}), req)
	if err != nil {
		t.Fatal(err)
	}
	nodes := literalTestNodes(draft)
	var triggerID string
	for _, edge := range draft.Edges {
		if edge.Kind == "leads_to" && nodes[edge.From].Label == "направления записи" {
			triggerID = edge.From
		}
	}
	for _, edge := range draft.Edges {
		if edge.To != triggerID || edge.Kind != "needs" || nodes[edge.From].Label != "оператор" {
			continue
		}
		if len(edge.Evidence) != 2 || edge.Evidence[0].Quote != story || edge.Evidence[1].Quote != criterion {
			t.Fatalf("expected the story and the separate acceptance criterion: %+v", edge)
		}
		for _, item := range edge.Evidence {
			if item.RequirementID != source.ID || item.Version != source.Version ||
				string([]rune(source.Text)[item.Start:item.End]) != item.Quote {
				t.Fatalf("expected evidence in the exact requirement version: %+v", item)
			}
		}
		return
	}
	t.Fatalf("the acceptance transition should be connected to the story role: %+v", draft)
}

func TestBuildKeepsAcceptanceTransitionSeparateWhenStoryContextIsAmbiguous(t *testing.T) {
	for _, text := range []string{
		"Я, как оператор, хочу редактировать список участников инспекции.\nПосле направления заявки список участников проверки фиксируется.",
		"Я, как оператор, хочу редактировать список участников.\nЯ, как контролёр, хочу видеть список участников.\nПосле направления заявки список участников фиксируется.",
	} {
		draft, err := Build(t.Context(), stubGenerator(func(context.Context, Request) (Suggestion, error) {
			return Suggestion{UsedSources: 1}, nil
		}), Request{Anchor: Anchor{Name: "участники"}, Sources: []Source{{ID: "story", Version: 1, Text: text}}})
		if err != nil {
			t.Fatal(err)
		}
		nodes := literalTestNodes(draft)
		for _, edge := range draft.Edges {
			if edge.Kind == "needs" && nodes[edge.To].Label == "направления заявки" {
				t.Fatalf("an ambiguous acceptance criterion was attributed to a role: %+v", draft)
			}
		}
	}
}
