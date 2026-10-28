package rag

import (
	"strings"
	"testing"
)

func TestSplitPreservesUnicodeSlicesAndProgress(t *testing.T) {
	text := strings.Repeat("Требование 👋 сохраняется. ", 150)
	runes := []rune(text)
	chunks := Split(text)
	if len(chunks) < 2 {
		t.Fatal("long requirement not split")
	}
	previous := -1
	for i, c := range chunks {
		if c.Start <= previous || c.End <= c.Start || c.End-c.Start > 1000 || c.End > len(runes) {
			t.Fatalf("bad chunk %d: %+v", i, c)
		}
		if string(runes[c.Start:c.End]) != c.Text {
			t.Fatalf("source changed in chunk %d", i)
		}
		if i > 0 && c.Start > chunks[i-1].End {
			t.Fatal("source gap")
		}
		previous = c.Start
	}
	if chunks[0].Start != 0 || chunks[len(chunks)-1].End != len(runes) {
		t.Fatal("source ends omitted")
	}
}
func TestSplitWhitespaceAndBoundaries(t *testing.T) {
	if got := Split(" \n\t "); len(got) != 0 {
		t.Fatal(got)
	}
	text := "Первый абзац.\n\nВторой абзац."
	got := Split(text)
	if len(got) != 1 || got[0].Text != text || got[0].Start != 0 || got[0].End != len([]rune(text)) {
		t.Fatal(got)
	}
	// A boundary near the cap is preferred while retaining overlap.
	text = strings.Repeat("а", 850) + ". " + strings.Repeat("б", 600)
	got = Split(text)
	if got[0].End != 851 {
		t.Fatalf("sentence boundary %d", got[0].End)
	}
	if got[1].Start != 751 {
		t.Fatalf("overlap start %d", got[1].Start)
	}
}
