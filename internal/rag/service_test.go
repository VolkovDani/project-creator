package rag

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestVectorsValidateDimensionsAndValues(t *testing.T) {
	good := make([]float32, 1024)
	good[0] = 1
	if _, err := vectorLiteral(good); err != nil {
		t.Fatal(err)
	}
	for _, v := range [][]float32{nil, make([]float32, 1024), {1, 2}} {
		if _, err := vectorLiteral(v); err == nil {
			t.Fatal("invalid vector accepted")
		}
	}
	bad := append([]float32(nil), good...)
	bad[1] = float32(math.Inf(1))
	if _, err := vectorLiteral(bad); err == nil {
		t.Fatal("infinity accepted")
	}
	bad[1] = float32(math.NaN())
	if _, err := vectorLiteral(bad); err == nil {
		t.Fatal("NaN accepted")
	}
}

type fakeEmbedder struct {
	profile string
	calls   int
	fail    bool
	started chan struct{}
	release chan struct{}
}

func (f *fakeEmbedder) Profile() string { return f.profile }
func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.started != nil {
		close(f.started)
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.fail {
		return nil, context.DeadlineExceeded
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 1024)
		axis := 0
		if strings.Contains(strings.ToLower(texts[i]), "banana") {
			axis = 1
		}
		out[i][axis] = 1
	}
	return out, nil
}
