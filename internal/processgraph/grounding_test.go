package processgraph

import "testing"

func TestGraphRequiresSpecificLabelsFromTheCitedSource(t *testing.T) {
	original := "Оператор открывает карточку. Затем проверяет реквизиты."
	unrelated := "Руководитель утверждает решение. Поля «следующий шаг» и «следующее действие» видны в карточке."
	req := Request{Sources: []Source{{ID: "first", Text: original, Version: 1}, {ID: "other", Text: unrelated, Version: 1}}}
	in := Suggestion{UsedSources: 2, Nodes: []Node{
		{ID: "a", Kind: "action", Label: "Открывает карточку"},
		{ID: "b", Kind: "action", Label: "Проверяет реквизиты"},
		{ID: "missing", Kind: "action", Label: "apply"},
		{ID: "combined", Kind: "action", Label: "Открывает реквизиты"},
		{ID: "placeholder", Kind: "action", Label: "Следующий шаг"},
		{ID: "placeholder2", Kind: "action", Label: "Следующее действие"},
		{ID: "other", Kind: "action", Label: "Утверждает решение"},
	}}
	first := SuggestedEvidence{Source: 1, Quote: original}
	other := SuggestedEvidence{Source: 2, Quote: unrelated}
	in.Edges = []SuggestedEdge{
		{From: "a", To: "b", Kind: "follows", Evidence: []SuggestedEvidence{first, other}},
		{From: "b", To: "a", Kind: "follows", Evidence: []SuggestedEvidence{first}},
		{From: "a", To: "missing", Kind: "leads_to", Evidence: []SuggestedEvidence{first}},
		{From: "a", To: "combined", Kind: "leads_to", Evidence: []SuggestedEvidence{first}},
		{From: "placeholder", To: "placeholder2", Kind: "follows", Evidence: []SuggestedEvidence{other}},
		{From: "a", To: "other", Kind: "triggers", Evidence: []SuggestedEvidence{first, other}},
	}
	draft, err := groundDraft(in, req, nil)
	if err != nil || len(draft.Edges) != 1 || draft.Edges[0].From != "a" || draft.Edges[0].To != "b" || len(draft.Edges[0].Evidence) != 1 || draft.Edges[0].Evidence[0].Quote != original {
		t.Fatalf("expected only the explicit source order: %+v (%v)", draft, err)
	}
}

func TestGraphGroundsSupportedGrammaticalForms(t *testing.T) {
	const quote = "Создание записи требует проверки."
	req := Request{Sources: []Source{{ID: "first", Text: quote, Version: 2}}}
	in := Suggestion{UsedSources: 1, Nodes: []Node{
		{ID: "create", Kind: "action", Label: "Создание записи"},
		{ID: "check", Kind: "rule", Label: "Требуется проверка"},
	}, Edges: []SuggestedEdge{
		{From: "create", To: "check", Kind: "requires", Evidence: []SuggestedEvidence{{Source: 1, Quote: quote}}},
	}}
	draft, err := groundDraft(in, req, nil)
	if err != nil || len(draft.Edges) != 1 || draft.Edges[0].Evidence[0].Quote != quote {
		t.Fatalf("expected matched grammatical forms and the original quote: %+v (%v)", draft, err)
	}
}

func TestGraphKeepsTheCompleteRoleInAUserStory(t *testing.T) {
	const story = "Я, как представитель заказчика, хочу, чтобы код отображался на карточке."
	const acceptance = "Код отображается на карточке."
	source := Source{ID: "story", Version: 2, Text: story + "\n" + acceptance}
	in := Suggestion{UsedSources: 1, Nodes: []Node{
		{ID: "representative", Kind: "actor", Label: "Представитель заказчика"},
		{ID: "customer", Kind: "actor", Label: "Заказчик"},
		{ID: "code", Kind: "state", Label: "Код отображался на карточке"},
	}}
	citations := []SuggestedEvidence{{Source: 1, Quote: story}, {Source: 1, Quote: acceptance}}
	in.Edges = []SuggestedEdge{
		{From: "customer", To: "code", Kind: "needs", Label: acceptance, Evidence: citations},
		{From: "representative", To: "code", Kind: "needs", Evidence: citations},
	}
	draft, err := groundDraft(in, Request{Sources: []Source{source}}, nil)
	if err != nil || len(draft.Edges) != 1 || draft.Edges[0].From != "representative" {
		t.Fatalf("expected the full actor stated in the story: %+v (%v)", draft, err)
	}
}
