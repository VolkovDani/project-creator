package api

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/assistantmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExternalMCPUsesAuthenticatedGraphQLAndBoundProject(t *testing.T) {
	tokenA, tokenB := strings.Repeat("a", 32), strings.Repeat("b", 32)
	s, err := New(testDatabase(t), map[string]string{tokenA: "a", tokenB: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'),('q','a','Q','G','','now','now'),('foreign','b','F','G','','now','now');
		INSERT INTO requirements VALUES ('r','a','p','visible synthetic source',1,'now','now'),
		 ('r1','a','p','second synthetic source',1,'now','now'),
		 ('q1','a','q','private sibling',1,'now','now'),('r2','b','foreign','private workspace',1,'now','now');
		INSERT INTO glossary_terms(id,workspace,project_id,name,definition,meaning_context,variants,version,source_id,source_version) VALUES
		 ('current','a','p','Synthetic Source','A fixture for the current project','Test data','[]',1,'',0),
		 ('second-a','a','p','Second Phase','Fixture A','','[{"text":"second","kind":"synonym"}]',1,'',0),
		 ('second-b','a','p','Second Stage','Fixture B','','[{"text":"second","kind":"synonym"}]',1,'',0),
		 ('private','a','q','Visible','A fixture from a sibling project','','[]',1,'',0);`); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(s)
	defer httpServer.Close()
	address, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(address.Port(), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	connect := func(token, projectID, sharing string) *mcp.ClientSession {
		t.Helper()
		server, err := assistantmcp.New(assistantmcp.Config{Port: uint16(port), Token: token, ProjectID: projectID, Sharing: sharing})
		if err != nil {
			t.Fatal(err)
		}
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		session, err := server.Connect(t.Context(), serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { session.Close() })
		client := mcp.NewClient(&mcp.Implementation{Name: "test-assistant", Version: "1"}, nil)
		connected, err := client.Connect(t.Context(), clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { connected.Close() })
		return connected
	}
	call := func(session *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	output := func(result *mcp.CallToolResult) string {
		t.Helper()
		value, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		return string(value)
	}
	authorized := connect(tokenA, "p", "local")
	tools, err := authorized.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"list_external_proposals", "list_requirements", "list_similar_requirement_pairs", "save_external_proposal"}) {
		t.Fatal(names)
	}
	first := call(authorized, "list_requirements", map[string]any{"first": 1})
	var page struct {
		PageInfo struct {
			EndCursor   string
			HasNextPage bool
		}
	}
	if first.IsError || !strings.Contains(output(first), "visible synthetic source") ||
		json.Unmarshal([]byte(output(first)), &page) != nil || !page.PageInfo.HasNextPage || page.PageInfo.EndCursor == "" {
		t.Fatal(output(first))
	}
	next := call(authorized, "list_requirements", map[string]any{"first": 1, "after": page.PageInfo.EndCursor})
	if next.IsError || !strings.Contains(output(next), "second synthetic source") {
		t.Fatal(output(next))
	}
	explicitScope := call(authorized, "list_requirements", map[string]any{"projectId": "q"})
	if !explicitScope.IsError {
		t.Fatal("tool accepted caller-supplied project", output(explicitScope))
	}
	for _, client := range []*mcp.ClientSession{connect(tokenB, "p", "local"), connect(strings.Repeat("x", 32), "p", "local")} {
		if result := call(client, "list_requirements", map[string]any{}); !result.IsError || strings.Contains(output(result), "synthetic") {
			t.Fatal("unauthorized data released", output(result))
		}
	}
	input := map[string]any{
		"requestId": "synthetic-1", "model": "synthetic-model", "analysis": "Check the examples",
		"limitations": "Synthetic only", "sources": []map[string]any{{"requirementId": "q1", "version": 1}},
	}
	if result := call(authorized, "save_external_proposal", input); !result.IsError || !strings.Contains(output(result), "CONFLICT") {
		t.Fatal("cross-project source accepted", output(result))
	}
	input["sources"] = []map[string]any{{"requirementId": "r", "version": 1}}
	input["glossarySources"] = []map[string]any{{"termId": "private", "version": 1}}
	if result := call(authorized, "save_external_proposal", input); !result.IsError || !strings.Contains(output(result), "glossarySources") {
		t.Fatal("cross-project glossary source accepted", output(result))
	}
	input["glossarySources"] = []map[string]any{{"termId": "current", "version": 1}}
	saved := call(authorized, "save_external_proposal", input)
	if saved.IsError || !strings.Contains(output(saved), "pending_review") ||
		!strings.Contains(output(saved), `"glossarySources":[{"status":"current","termId":"current","version":1}]`) ||
		!strings.Contains(output(saved), "test-assistant 1") || !strings.Contains(output(saved), `"sharing":"local"`) {
		t.Fatal(output(saved), saved.Content)
	}
	if result := call(authorized, "list_external_proposals", map[string]any{}); result.IsError ||
		!strings.Contains(output(result), "Check the examples") ||
		!strings.Contains(output(result), `"glossarySources":[{"status":"current","termId":"current","version":1}]`) {
		t.Fatal(output(result))
	}
	checkSimilarMCPPairs(t, s, call, authorized, connect(tokenB, "p", "local"))
	checkSparseMCPPairs(t, s, call, connect(tokenA, "sparse", "local"))
}

func checkSimilarMCPPairs(
	t *testing.T, s *Server,
	call func(*mcp.ClientSession, string, any) *mcp.CallToolResult,
	authorized, unauthorized *mcp.ClientSession,
) {
	t.Helper()
	const tool = "list_similar_requirement_pairs"
	type source struct {
		ID, Text string
		Version  int
	}
	type page struct {
		Status, EmbeddingProfile string
		MissingRequirements      int
		Items                    []struct {
			Left, Right     source
			SimilarityScore float64
			GlossaryContext []struct {
				TermID, Spelling, Name, Definition, Context string
				Version                                     int
				Ambiguous                                   bool
			}
		}
		PageInfo struct {
			EndCursor   string
			HasNextPage bool
		}
	}
	result := func(first int, threshold float64, after string) page {
		t.Helper()
		args := map[string]any{"first": first, "threshold": threshold}
		if after != "" {
			args["after"] = after
		}
		response := call(authorized, tool, args)
		var p page
		raw, err := json.Marshal(response.StructuredContent)
		if err != nil || json.Unmarshal(raw, &p) != nil || p.Status == "" || (p.Status == "ready") == response.IsError {
			t.Fatalf("unexpected tool response: %s %v", raw, response.Content)
		}
		return p
	}
	if got := result(1, 0.8, ""); got.Status != "embeddings_not_configured" || len(got.Items) != 0 {
		t.Fatal(got)
	}
	if _, err := s.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at,archived_at) VALUES
		 ('s','a','p','another similar requirement',1,'now','now',NULL),
		 ('t','a','p','visible synthetic source',1,'now','now',NULL),
		 ('archived','a','p','archived requirement',1,'now','now','now');
		UPDATE embedding_profiles SET profile='fixture' WHERE workspace='a';
		INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until)
		SELECT workspace,project_id,id,'fixture',version,'ready','fixture','now' FROM requirements;
		INSERT INTO requirement_chunks(workspace,project_id,requirement_id,profile,version,ordinal,start_offset,end_offset,text,embedding)
		SELECT r.workspace,r.project_id,r.id,'fixture',r.version,c.ordinal,0,1,r.text,
		 ('[' || c.components || repeat(',0',1022) || ']')::public.vector
		FROM requirements r JOIN (VALUES
		  ('r',0,'0,1'),('r',1,'1,0'),('r1',0,'1,0'),('s',0,'0.9,0.1'),
		  ('archived',0,'1,0'),('q1',0,'1,0'),('r2',0,'1,0')
		) c(id,ordinal,components) ON c.id=r.id;
	`); err != nil {
		t.Fatal(err)
	}
	if got := result(1, 0.8, ""); got.Status != "index_incomplete" || got.MissingRequirements != 1 || len(got.Items) != 0 {
		t.Fatal(got)
	}
	if _, err := s.db.Exec(`INSERT INTO requirement_chunks(workspace,project_id,requirement_id,profile,version,ordinal,start_offset,end_offset,text,embedding)
		VALUES ('a','p','t','fixture',1,0,0,1,'visible synthetic source', ('[0,-1' || repeat(',0',1022) || ']')::public.vector)`); err != nil {
		t.Fatal(err)
	}

	first := result(1, 0.8, "")
	if first.Status != "ready" || first.EmbeddingProfile != "fixture" || len(first.Items) != 1 || !first.PageInfo.HasNextPage || first.PageInfo.EndCursor == "" {
		t.Fatal(first)
	}
	pair := first.Items[0]
	if pair.Left != (source{ID: "r", Text: "visible synthetic source", Version: 1}) || pair.Right != (source{ID: "r1", Text: "second synthetic source", Version: 1}) || pair.SimilarityScore != 1 {
		t.Fatal(pair)
	}
	if len(pair.GlossaryContext) != 2 {
		t.Fatal("expected only the current project's glossary context", pair.GlossaryContext)
	}
	ambiguous := pair.GlossaryContext[0]
	if ambiguous.Spelling != "second" || !ambiguous.Ambiguous || ambiguous.TermID != "" || ambiguous.Version != 0 ||
		ambiguous.Name != "" || ambiguous.Definition != "" || ambiguous.Context != "" {
		t.Fatal("ambiguous spelling should have no chosen term", ambiguous)
	}
	hint := pair.GlossaryContext[1]
	if hint.TermID != "current" || hint.Version != 1 || hint.Spelling != "synthetic source" ||
		hint.Name != "Synthetic Source" || hint.Definition != "A fixture for the current project" ||
		hint.Context != "Test data" || hint.Ambiguous {
		t.Fatal(hint)
	}
	next := result(20, 0.8, first.PageInfo.EndCursor)
	if next.Status != "ready" || len(next.Items) != 2 || next.PageInfo.HasNextPage {
		t.Fatal(next)
	}
	for i, pair := range next.Items {
		if pair.Left.ID != []string{"r", "r1"}[i] || pair.Right.ID != "s" || pair.SimilarityScore < 0.8 || pair.SimilarityScore >= 1 {
			t.Fatal(pair)
		}
	}
	zeroCursor := result(1, 0, "").PageInfo.EndCursor
	if got := result(20, math.Copysign(0, -1), zeroCursor); got.Status != "ready" {
		t.Fatal("equivalent zero threshold rejected", got)
	}
	atBoundary := result(20, 1, "")
	if atBoundary.Status != "ready" || len(atBoundary.Items) != 1 || atBoundary.Items[0].SimilarityScore != 1 {
		t.Fatal(atBoundary)
	}
	for _, args := range []map[string]any{
		{"threshold": 1.01}, {"threshold": 0.8, "projectId": "q"},
		{"first": 1, "threshold": 0.9, "after": first.PageInfo.EndCursor},
	} {
		if got := call(authorized, tool, args); !got.IsError {
			t.Fatal("invalid input accepted", args)
		}
	}
	if got := call(unauthorized, tool, map[string]any{"threshold": 0.8}); !got.IsError || got.StructuredContent != nil {
		t.Fatal("cross-workspace access", got)
	}
	if _, err := s.db.Exec(`UPDATE glossary_terms SET version=version+1 WHERE id='private'`); err != nil {
		t.Fatal(err)
	}
	if got := result(20, 0.8, first.PageInfo.EndCursor); got.Status != "ready" {
		t.Fatal("sibling glossary changed the cursor", got)
	}
	if _, err := s.db.Exec(`UPDATE glossary_terms SET definition='Revised fixture',version=2 WHERE id='current'`); err != nil {
		t.Fatal(err)
	}
	if got := result(20, 0.8, first.PageInfo.EndCursor); got.Status != "cursor_stale" || len(got.Items) != 0 {
		t.Fatal(got)
	}
	restarted := result(1, 0.8, "")
	if hints := restarted.Items[0].GlossaryContext; len(hints) != 2 || hints[1].Version != 2 || hints[1].Definition != "Revised fixture" {
		t.Fatal(hints)
	}
	if _, err := s.db.Exec(`UPDATE requirement_index SET attempt='replacement' WHERE requirement_id='s' AND profile='fixture'`); err != nil {
		t.Fatal(err)
	}
	if got := result(20, 0.8, restarted.PageInfo.EndCursor); got.Status != "cursor_stale" || len(got.Items) != 0 {
		t.Fatal(got)
	}
	newCursor := result(1, 0.8, "").PageInfo.EndCursor
	if _, err := s.db.Exec(`UPDATE requirements SET text='updated',version=2 WHERE id='s'`); err != nil {
		t.Fatal(err)
	}
	if got := result(20, 0.8, newCursor); got.Status != "cursor_stale" || len(got.Items) != 0 {
		t.Fatal(got)
	}
	var runs int
	if err := s.db.QueryRow(`SELECT count(*) FROM analysis_runs`).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("MCP read started an analysis run: %d, %v", runs, err)
	}
}

func checkSparseMCPPairs(
	t *testing.T, s *Server,
	call func(*mcp.ClientSession, string, any) *mcp.CallToolResult,
	client *mcp.ClientSession,
) {
	t.Helper()
	if _, err := s.db.Exec(`
		INSERT INTO projects VALUES ('sparse','a','Sparse','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		 SELECT 's'||lpad(n::text,2,'0'),'a','sparse','Fixture '||n,1,'now','now' FROM generate_series(1,18) n;
		UPDATE requirement_index SET status='ready',attempt='test' WHERE project_id='sparse';
		INSERT INTO requirement_chunks(workspace,project_id,requirement_id,profile,version,ordinal,start_offset,end_offset,text,embedding)
		 SELECT workspace,project_id,requirement_id,profile,version,0,0,1,'fixture',
		  ARRAY(SELECT (d=least(right(requirement_id,2)::int,17))::int FROM generate_series(1,1024) d)::public.vector
		 FROM requirement_index WHERE project_id='sparse' AND status='ready';
	`); err != nil {
		t.Fatal(err)
	}
	type page struct {
		Items []struct {
			Left, Right struct{ ID string }
		}
		PageInfo struct {
			EndCursor   string
			HasNextPage bool
		}
	}
	read := func(args map[string]any) page {
		t.Helper()
		got := call(client, "list_similar_requirement_pairs", args)
		var decoded page
		raw, err := json.Marshal(got.StructuredContent)
		if got.IsError || err != nil || json.Unmarshal(raw, &decoded) != nil {
			t.Fatalf("sparse page failed: %s %v", raw, got.Content)
		}
		return decoded
	}
	first := read(map[string]any{"threshold": 0.8})
	if len(first.Items) != 0 || !first.PageInfo.HasNextPage || first.PageInfo.EndCursor == "" {
		t.Fatal("expected an empty page with a continuation", first)
	}
	last := read(map[string]any{"threshold": 0.8, "after": first.PageInfo.EndCursor})
	if len(last.Items) != 1 || last.PageInfo.HasNextPage || last.Items[0].Left.ID != "s17" || last.Items[0].Right.ID != "s18" {
		t.Fatal("expected the only match on the last page", last)
	}
}
