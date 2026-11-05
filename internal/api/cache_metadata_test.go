package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCacheTagsUseOnlyAuthorizedResults(t *testing.T) {
	data := map[string]any{"project": map[string]any{"id": "p", "goal": "g"}}
	want := []string{"project:p", "project:p:requirements", "projects"}
	if got := cacheTags(data); !reflect.DeepEqual(got, want) {
		t.Fatalf("tags %v", got)
	}
	if got := cacheTags(map[string]any{"project": nil}); !reflect.DeepEqual(got, []string{"projects"}) {
		t.Fatal(got)
	}
}
func TestRequirementIndexResultsCarryProjectCacheTags(t *testing.T) {
	data := map[string]any{"requirementIndexByIds": []any{
		map[string]any{"requirementId": "r", "projectId": "p", "version": float64(1), "status": "ready"},
	}}
	want := []string{"project:p", "project:p:requirements", "projects"}
	if got := cacheTags(data); !reflect.DeepEqual(got, want) {
		t.Fatalf("tags %v", got)
	}
}
func TestAnalysisSummaryAndHistoryUseAnalysisTagOnly(t *testing.T) {
	for _, data := range []map[string]any{
		{"projectAnalysis": map[string]any{"id": "r", "projectId": "p", "revision": "3", "mode": "all", "status": "running"}},
		{"scope": map[string]any{"__typename": "ProjectAnalysis", "projectId": "p"}},
		{"analysisHistory": []any{map[string]any{"id": "r", "projectId": "p", "mode": "all", "createdAt": "now"}}},
	} {
		if got := cacheTags(data); !reflect.DeepEqual(got, []string{"project:p:analysis"}) {
			t.Fatalf("analysis tags %v", got)
		}
	}
}
func TestCacheTagsKeepProjectListForMixedAnalysisQueries(t *testing.T) {
	data := map[string]any{
		"projects": []any{map[string]any{"id": "p", "goal": "g"}},
		"analysis": map[string]any{"id": "r", "projectId": "p", "revision": "2", "mode": "all", "status": "running"},
	}
	want := []string{"project:p", "project:p:analysis", "project:p:requirements", "projects"}
	if got := cacheTags(data); !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed tags %v", got)
	}
}
func TestTaggedResponseIsNotServerCached(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"projects":[]}}`))
	})
	w := httptest.NewRecorder()
	serveTaggedGraphQL(h, w, httptest.NewRequest("POST", "/graphql", nil))
	var payload struct {
		Extensions struct {
			CacheTags []string `json:"cacheTags"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Cache-Control") != "no-store" || !reflect.DeepEqual(payload.Extensions.CacheTags, []string{"projects"}) {
		t.Fatalf("%v %s", w.Header(), w.Body.String())
	}
}
func TestEventRouteRequiresAuthorization(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/events", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	req := httptest.NewRequest("POST", "/events", nil)
	req.Header.Set("Authorization", "Bearer token-a")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal(w.Code)
	}
}
