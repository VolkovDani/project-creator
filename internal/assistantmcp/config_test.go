package assistantmcp

import (
	"strings"
	"testing"
)

func TestMCPRequiresExplicitProjectSharingAndWorkspaceToken(t *testing.T) {
	valid := Config{Port: 8080, Token: strings.Repeat("x", 32), ProjectID: "p", Sharing: "local"}
	for _, cfg := range []Config{
		{Port: valid.Port, Token: valid.Token, ProjectID: valid.ProjectID},
		{Port: valid.Port, Token: valid.Token, Sharing: "local"},
		{Port: valid.Port, Token: valid.Token, ProjectID: valid.ProjectID, Sharing: "yes"},
		{Port: valid.Port, Token: "missing-or-short", ProjectID: valid.ProjectID, Sharing: valid.Sharing},
		{Token: valid.Token, ProjectID: valid.ProjectID, Sharing: valid.Sharing},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatal("unsafe MCP configuration accepted")
		}
	}
	for _, mode := range []string{"local", "cloud"} {
		valid.Sharing = mode
		if _, err := New(valid); err != nil {
			t.Fatalf("explicit %s sharing rejected: %v", mode, err)
		}
	}
}
