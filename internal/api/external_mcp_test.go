package api

import (
	"encoding/json"
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
		 ('q1','a','q','private sibling',1,'now','now'),('r2','b','foreign','private workspace',1,'now','now');`); err != nil {
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
	if !slices.Equal(names, []string{"list_external_proposals", "list_requirements", "save_external_proposal"}) {
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
	saved := call(authorized, "save_external_proposal", input)
	if saved.IsError || !strings.Contains(output(saved), "pending_review") ||
		!strings.Contains(output(saved), "test-assistant 1") || !strings.Contains(output(saved), `"sharing":"local"`) {
		t.Fatal(output(saved), saved.Content)
	}
	if result := call(authorized, "list_external_proposals", map[string]any{}); result.IsError ||
		!strings.Contains(output(result), "Check the examples") {
		t.Fatal(output(result))
	}
}
