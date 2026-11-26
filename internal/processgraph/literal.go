package processgraph

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const clauseStart = `(?im)(?:^\s*(?:[-*]\s+|\d+[.)]\s+)?|[.!?]\s+)`
const horizontalSpace = `[\p{Zs}\t]`

// Match a known action form and an explicit separate result clause. With no
// separating comma, require a recognizable result subject and predicate.
const temporalPrefix = `(?:в` + horizontalSpace + `+[^,.!?;\r\n]{1,60}?` + horizontalSpace + `+)?`
const temporalActionRU = `(?:нажатии|нажатия|дублировании|дублирования|копировании|копирования|направлении|направления|подписании|подписания|сохранении|сохранения|создании|создания)`
const temporalActionEN = `(?:clicking|pressing|duplicating|copying|sending|signing|saving|creating|(?:I|we|you)` + horizontalSpace + `+(?:click|press|duplicate|copy|send|sign|save|create))`
const actionObject = `(?:` + horizontalSpace + `+[^,.!?;\r\n]+?)??`
const temporalMarker = `(?:при|после|when|upon|after)` + horizontalSpace + `+`
const russianResultVerb = `(?:(?:устанавлива|меня|счита|фиксиру|блокиру|отобража|открыва|закрыва|появля|сохраня|создава)` +
	`(?:ется|ются|лся|лась|лось|лись)|(?:станов|переход)(?:ится|ятся|ился|илась|илось|ились|ит|ят|ил|ила|ило|или)|` +
	`создаётся|создается|создаются|откроется|появится|сохраняет|получает|получала)`
const englishResultVerb = `(?:appears|closes|changes|displays|locks|moves|opens|shows|starts|becomes|` +
	`(?:is|are|was|were)` + horizontalSpace + `+(?:archived|closed|created|displayed|locked|opened|saved|shown))`
const temporalEffect = temporalPrefix + temporalMarker +
	`((?:` + temporalActionRU + `|` + temporalActionEN + `)` + actionObject + `),` + horizontalSpace + `+([^.!?;\r\n]+)`
const unpunctuatedEffect = temporalPrefix + `(?:при|после)` + horizontalSpace + `+(` + temporalActionRU + actionObject + `)` + horizontalSpace +
	`+((?:статус|результат|список|(?:регистрационный` + horizontalSpace + `+)?номер|форма|поле|запись)` + horizontalSpace + `+` +
	`(?:[^,.!?;\r\n]{1,60}?` + horizontalSpace + `+)?` +
	russianResultVerb + `(?:[\p{Zs}\t,]+[^.!?;\r\n]+)?)`

var temporalClause = regexp.MustCompile(clauseStart + `(` + temporalPrefix + temporalMarker + `[^.!?;\r\n]+)`)
var finiteResult = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(?:` + russianResultVerb + `|` + englishResultVerb + `)(?:$|[^\p{L}])`)
var modalTrigger = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(?:можно|мог|могла|могли|могу|может|можем|можете|могут|can|could|may)(?:$|[^\p{L}])`)

var temporalEffects = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^` + unpunctuatedEffect + `$`),
	regexp.MustCompile(`(?i)^` + temporalEffect + `$`),
}

type literalPattern struct {
	clause           *regexp.Regexp
	fromKind, toKind string
	relationship     string
}

// Recover directly stated clauses without inferring any ordering between them.
var literalPatterns = []literalPattern{
	{
		clause:   regexp.MustCompile(clauseStart + `((?:если|if)[\p{Zs}\t]+([^,.!?;\r\n]+),[\p{Zs}\t]+(?:(?:то|then)[\p{Zs}\t]+)?([^.!?;\r\n]+))`),
		fromKind: "condition", toKind: "event", relationship: "leads_to",
	},
	{
		clause:   regexp.MustCompile(clauseStart + `(([^,.!?;\r\n]+?)[\p{Zs}\t]+(?:запрещено|не может|не вправе|не имеет права|cannot|may not|must not|is prohibited from)[\p{Zs}\t]+([^.!?;\r\n]+))`),
		fromKind: "actor", toKind: "action", relationship: "prohibits",
	},
	{
		clause: regexp.MustCompile(clauseStart + `((?:я[\p{Zs}	,]+)?как[\p{Zs}	]+([^,
]+?)[\p{Zs}	,]+(?:я[\p{Zs}	]+)?(?:хочу|долж(?:ен|на|ны)[\p{Zs}	]+иметь[\p{Zs}	]+возможность)[\p{Zs}	,]+([^.!?;
]+))`),
		fromKind: "actor", toKind: "action", relationship: "needs",
	},
	{
		clause: regexp.MustCompile(clauseStart + `(as[\p{Zs}	]+(?:an?[\p{Zs}	]+)?([^,
]+?)[\p{Zs}	,]+(?:i|we)[\p{Zs}	]+(?:want|need)[\p{Zs}	]+(?:to|that)[\p{Zs}	]+([^.!?;
]+))`),
		fromKind: "actor", toKind: "action", relationship: "needs",
	},
}

type literalLink struct {
	from, to Node
	kind     string
}

type literalClause struct {
	source Source
	quote  string
	start  int
	links  []literalLink
	story  *literalStory
}

type literalStory struct {
	role   Node
	action string
	quote  string
	start  int
}

func (c literalClause) suggestion() Suggestion {
	result := Suggestion{UsedSources: 1}
	nodeIDs := map[[2]string]string{}
	node := func(value Node) string {
		key := [2]string{value.Kind, strings.ToLower(value.Label)}
		id, found := nodeIDs[key]
		if !found {
			id = strconv.Itoa(len(result.Nodes) + 1)
			nodeIDs[key] = id
			value.ID = id
			result.Nodes = append(result.Nodes, value)
		}
		return id
	}
	for _, link := range c.links {
		result.Edges = append(result.Edges, SuggestedEdge{
			From: node(link.from), To: node(link.to), Kind: link.kind,
			Evidence: []SuggestedEvidence{{Source: 1, Start: &c.start, Quote: c.quote}},
		})
	}
	return result
}

// Prefer chains and explicit transitions across the selected scope before
// standalone user requests. Keep each chain together under the literal limit.
func explicitLiteralClauses(sources []Source) []literalClause {
	var chains, explicit, needs []literalClause
	for _, source := range sources {
		var transitions, rules []literalClause
		var stories []literalStory
		for _, match := range temporalClause.FindAllStringSubmatchIndex(source.Text, -1) {
			clause := citedLiteralClause(source, match[2], match[3])
			if link, ok := explicitTemporalLink(beforeStoryPurpose(clause.quote)); ok {
				clause.links = []literalLink{link}
				transitions = append(transitions, clause)
			}
		}
		for _, pattern := range literalPatterns {
			for _, match := range pattern.clause.FindAllStringSubmatchIndex(source.Text, -1) {
				left := strings.TrimSpace(source.Text[match[4]:match[5]])
				right := strings.TrimSpace(source.Text[match[6]:match[7]])
				if !validLabel(left, false) {
					continue
				}
				clause := citedLiteralClause(source, match[2], match[3])
				if pattern.relationship != "needs" {
					right = beforeStoryPurpose(right)
					if validLabel(right, false) && (pattern.relationship != "leads_to" || explicitResult(right)) {
						clause.links = []literalLink{{from: Node{Kind: pattern.fromKind, Label: left}, to: Node{Kind: pattern.toKind, Label: right}, kind: pattern.relationship}}
						rules = append(rules, clause)
					}
					continue
				}
				right = storyAction(right)
				role := Node{Kind: "actor", Label: left}
				stories = append(stories, literalStory{role: role, action: right, quote: clause.quote, start: clause.start})
				if transition, ok := explicitTemporalLink(right); ok {
					clause.links = []literalLink{{from: role, to: transition.from, kind: "needs"}, transition}
					chains = append(chains, clause)
				} else if validLabel(right, false) {
					clause.links = []literalLink{{from: role, to: Node{Kind: "action", Label: right}, kind: "needs"}}
					needs = append(needs, clause)
				}
			}
		}
		if len(stories) == 1 {
			story := &stories[0]
			for i, transition := range transitions {
				if story.covers(transition) {
					transitions[i].story = story
				}
			}
		}
		explicit = append(explicit, transitions...)
		explicit = append(explicit, rules...)
	}
	return slices.Concat(chains, explicit, needs)
}

// An acceptance criterion can describe a change to the same named field
// requested in a preceding story. Require the full result subject and
// a single story in the requirement before connecting the two clauses.
func (s *literalStory) covers(clause literalClause) bool {
	result := clause.links[0].to.Label
	predicate := finiteResult.FindStringIndex(result)
	if predicate == nil || clause.start < s.start+utf8.RuneCountInString(s.quote) {
		return false
	}
	subject := labelWords(result[:predicate[0]])
	return len(subject) >= 2 && sourceLabelPosition(subject, labelWords(s.action)) >= 0
}

func (c literalClause) attachStory(draft *Draft, proof *conceptProof) {
	if c.story == nil || len(draft.Edges) != 1 {
		return
	}
	transition := draft.Edges[0]
	index := slices.IndexFunc(draft.Nodes, func(node Node) bool { return node.ID == transition.From })
	evidence := groundedEvidence([]SuggestedEvidence{{Source: 1, Start: &c.story.start, Quote: c.story.quote}}, []Source{c.source}, "")
	if index < 0 || len(evidence) == 0 {
		return
	}
	evidence = append(evidence, transition.Evidence...)
	role := c.story.role
	if !proof.supports(role, draft.Nodes[index], evidence) {
		return
	}
	role.ID = strconv.Itoa(len(draft.Nodes) + 1)
	draft.Nodes = append(draft.Nodes, role)
	draft.Edges = append(draft.Edges, Edge{From: role.ID, To: transition.From, Kind: "needs", Evidence: evidence})
}

func citedLiteralClause(source Source, start, end int) literalClause {
	return literalClause{source: source, quote: strings.TrimSpace(source.Text[start:end]), start: utf8.RuneCountInString(source.Text[:start])}
}

func explicitTemporalLink(text string) (literalLink, bool) {
	for _, pattern := range temporalEffects {
		if match := pattern.FindStringSubmatch(text); match != nil {
			action, result := strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
			mainClause, _, _ := strings.Cut(result, ",")
			if validLabel(action, false) && validLabel(result, false) && !modalTrigger.MatchString(action) &&
				!finiteResult.MatchString(action) && explicitResult(result) && finiteResult.MatchString(mainClause) {
				return literalLink{from: Node{Kind: "action", Label: action}, to: Node{Kind: "event", Label: result}, kind: "leads_to"}, true
			}
		}
	}
	return literalLink{}, false
}

func augmentLiteralDraft(draft *Draft, req Request, proof *conceptProof) bool {
	model := *draft
	modelNodes := make(map[string]Node, len(model.Nodes))
	for _, node := range model.Nodes {
		modelNodes[node.ID] = node
	}
	added := 0
	for _, clause := range explicitLiteralClauses(req.Sources) {
		sourceReq := Request{Anchor: req.Anchor, Sources: []Source{clause.source}}
		literal, err := groundDraft(clause.suggestion(), sourceReq, proof)
		if err != nil {
			continue
		}
		extracted := len(literal.Edges)
		clause.attachStory(&literal, proof)
		retainLiteralEdges(&literal, model, modelNodes)
		// A story link adds context to an extracted transition within the same unit of work.
		count := min(extracted, len(literal.Edges))
		if count == 0 || added+count > MaxEdges {
			continue
		}
		mergeDraft(draft, literal)
		added += count
		if added == MaxEdges {
			break
		}
	}
	if added == 0 {
		return false
	}
	draft.Partial = true
	draft.SourceCount = len(req.Sources)
	return true
}

func retainLiteralEdges(literal *Draft, model Draft, modelNodes map[string]Node) {
	nodes := make(map[string]Node, len(literal.Nodes))
	for _, node := range literal.Nodes {
		nodes[node.ID] = node
	}
	used := make(map[string]bool)
	literal.Edges = slices.DeleteFunc(literal.Edges, func(edge Edge) bool {
		from, to := nodes[edge.From], nodes[edge.To]
		for _, existing := range model.Edges {
			modelFrom, modelTo := modelNodes[existing.From], modelNodes[existing.To]
			if edge.Kind == existing.Kind && sameLiteralOccurrence(edge.Evidence[0], existing.Evidence) &&
				supportsLabel(existing.Label, edge.Evidence[0].Quote) &&
				compatibleLiteralLabel(from.Label, modelFrom.Label) && compatibleLiteralLabel(to.Label, modelTo.Label) {
				// Keep the shared node when the literal citation also supports its model label.
				quote := labelWords(edge.Evidence[0].Quote)
				for _, pair := range [][2]Node{{from, modelFrom}, {to, modelTo}} {
					if pair[0].Kind == pair[1].Kind && sourceLabelPosition(labelWords(pair[1].Label), quote) >= 0 {
						nodes[pair[0].ID] = Node{ID: pair[0].ID, Kind: pair[1].Kind, Label: pair[1].Label}
					}
				}
				return true
			}
		}
		used[edge.From], used[edge.To] = true, true
		return false
	})
	literal.Nodes = slices.DeleteFunc(literal.Nodes, func(node Node) bool { return !used[node.ID] })
	for i, node := range literal.Nodes {
		literal.Nodes[i] = nodes[node.ID]
	}
}

func compatibleLiteralLabel(left, right string) bool {
	leftWords, rightWords := labelWords(left), labelWords(right)
	negated := func(words []string) bool {
		return slices.ContainsFunc(words, func(word string) bool {
			return slices.Contains([]string{"не", "без", "not", "no", "never", "without"}, word)
		})
	}
	return negated(leftWords) == negated(rightWords) &&
		(sourceLabelPosition(leftWords, rightWords) >= 0 || sourceLabelPosition(rightWords, leftWords) >= 0)
}

func sameLiteralOccurrence(literal Evidence, model []Evidence) bool {
	return slices.ContainsFunc(model, func(existing Evidence) bool {
		return literal.RequirementID == existing.RequirementID && literal.Version == existing.Version &&
			literal.Start < existing.End && existing.Start < literal.End
	})
}

var storyActionStarter = regexp.MustCompile(`(?i)^чтобы[\p{Zs}	]+`)
var storyPurpose = regexp.MustCompile(`(?i)(?:,[\p{Zs}	]*чтобы|(?:,[\p{Zs}	]*|[\p{Zs}	]+)для[\p{Zs}	]+того[\p{Zs}	,]+чтобы|[\p{Zs}	]+(?:so[\p{Zs}	]+that|in[\p{Zs}	]+order[\p{Zs}	]+to))[\p{Zs}	]+`)
var dependentResult = regexp.MustCompile(`(?i)^(?:чтобы|если|if|(?:к|для|у|о|в)[\p{Zs}\t]+котор\p{L}*|котор\p{L}*|which|whom|whose|для[\p{Zs}\t]+того[\p{Zs}\t,]+чтобы|so[\p{Zs}\t]+that|in[\p{Zs}\t]+order[\p{Zs}\t]+to)[\p{Zs}\t]+`)

func explicitResult(text string) bool { return !dependentResult.MatchString(text) }

func storyAction(text string) string {
	if lead := storyActionStarter.FindStringIndex(text); lead != nil {
		text = text[lead[1]:]
	}
	return beforeStoryPurpose(text)
}

func beforeStoryPurpose(text string) string {
	if purpose := storyPurpose.FindStringIndex(text); purpose != nil {
		text = text[:purpose[0]]
	}
	return strings.TrimSpace(text)
}
