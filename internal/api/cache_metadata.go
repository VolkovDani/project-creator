package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
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
func cacheTags(data any) []string {
	tags := map[string]bool{"projects": true}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if id, ok := x["projectId"].(string); ok {
				tags["project:"+id] = true
				tags["project:"+id+":requirements"] = true
			}
			if _, ok := x["goal"]; ok {
				if id, ok := x["id"].(string); ok {
					tags["project:"+id] = true
					tags["project:"+id+":requirements"] = true
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
	out := make([]string, 0, len(tags))
	for k := range tags {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func serveTaggedGraphQL(h http.Handler, w http.ResponseWriter, r *http.Request) {
	buffered := &bufferedResponse{header: make(http.Header)}
	h.ServeHTTP(buffered, r)
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
		ext["cacheTags"] = cacheTags(payload["data"])
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
