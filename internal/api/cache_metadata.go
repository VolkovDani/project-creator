package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/VolkovDani/project-creator/graph"
)

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}

// Projects is a conservative workspace-scoped dependency for every current
// operation. Entity tags allow finer subscriptions as the API grows. IDs in
// tags come only from the authorized result, never from unchecked variables.
func cacheTags(data any, dependencies ...string) []string {
	tags := map[string]bool{}
	for _, tag := range dependencies {
		tags[tag] = true
	}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if id, ok := x["projectId"].(string); ok {
				switch x["__typename"] {
				case "RequirementClassification", "ClassificationListItem":
					if record, ok := x["requirementId"].(string); ok {
						tags["project:"+id+":classification:"+record] = true
						return
					}
				case "ClassificationGroup", "ClassificationProgress":
					tags["project:"+id+":classifications"] = true
					return
				}
				_, hasRevision := x["revision"].(string)
				_, hasAnalysisMode := x["mode"].(string)
				_, hasCreatedAt := x["createdAt"].(string)
				_, hasGeneratorProfile := x["generatorProfile"].(string)
				_, hasGeneratorProvenance := x["generatorProvenance"].(string)
				isAnalysis := x["__typename"] == "ProjectAnalysis" || x["__typename"] == "ProjectAnalysisSummary" || (hasRevision && hasAnalysisMode) || (hasAnalysisMode && hasCreatedAt) || (hasGeneratorProfile && hasGeneratorProvenance)
				_, hasSummaryProvenance := x["provenance"].(string)
				if hasSummaryProvenance {
					tags["project:"+id+":summaries"] = true
				}
				if isAnalysis {
					tags["project:"+id+":analysis"] = true
				} else {
					tags["project:"+id] = true
					tags["project:"+id+":requirements"] = true
					tags["projects"] = true
				}
			}
			if _, ok := x["goal"]; ok {
				if id, ok := x["id"].(string); ok {
					tags["project:"+id] = true
					tags["project:"+id+":requirements"] = true
					tags["projects"] = true
				}
			}
			for _, v := range x {
				visit(v)
			}
		case []any:
			for _, v := range x {
				visit(v)
			}
		}
	}
	visit(data)
	if len(tags) == 0 {
		tags["projects"] = true
	}
	out := make([]string, 0, len(tags))
	for k := range tags {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func serveTaggedGraphQL(h http.Handler, w http.ResponseWriter, r *http.Request) {
	buffered := &bufferedResponse{header: make(http.Header)}
	ctx, dependencies := graph.WithCacheDependencies(r.Context())
	h.ServeHTTP(buffered, r.WithContext(ctx))
	for key, values := range buffered.header {
		w.Header()[key] = values
	}
	w.Header().Set("Cache-Control", "no-store")
	var payload map[string]any
	body := buffered.body.Bytes()
	if json.Unmarshal(body, &payload) == nil && payload["data"] != nil {
		ext, ok := payload["extensions"].(map[string]any)
		if !ok {
			ext = map[string]any{}
		}
		ext["cacheTags"] = cacheTags(payload["data"], dependencies()...)
		payload["extensions"] = ext
		if encoded, err := json.Marshal(payload); err == nil {
			body = encoded
		}
	}
	status := buffered.status
	if status == 0 {
		status = 200
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
