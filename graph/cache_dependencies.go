package graph

import (
	"context"
	"sync"
)

type dependenciesKey struct{}
type dependencies struct {
	mu   sync.Mutex
	tags map[string]bool
}

// WithCacheDependencies lets a resolver register the scope it actually read,
// including authorized list queries that returned no rows.
func WithCacheDependencies(ctx context.Context) (context.Context, func() []string) {
	deps := &dependencies{tags: make(map[string]bool)}
	return context.WithValue(ctx, dependenciesKey{}, deps), func() []string {
		deps.mu.Lock()
		defer deps.mu.Unlock()
		out := make([]string, 0, len(deps.tags))
		for tag := range deps.tags {
			out = append(out, tag)
		}
		return out
	}
}

func addCacheDependency(ctx context.Context, tag string) {
	deps, ok := ctx.Value(dependenciesKey{}).(*dependencies)
	if ok {
		deps.mu.Lock()
		deps.tags[tag] = true
		deps.mu.Unlock()
	}
}
