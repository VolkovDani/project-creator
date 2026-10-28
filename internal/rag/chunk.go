package rag

import (
	"strings"
	"unicode"
)

// Chunk offsets count Unicode code points in the unchanged source text.
type Chunk struct {
	Text       string
	Start, End int
}

// Split uses bounded source slices, preferring paragraph/sentence endings in the
// latter half of a window. Adjacent chunks overlap by 100 code points. It never
// joins requirements or normalizes their text; limits are a versioned heuristic.
func Split(text string) []Chunk {
	source := []rune(text)
	out := []Chunk{}
	for start := 0; start < len(source); {
		end := start + 1000
		if end >= len(source) {
			end = len(source)
		} else {
			minimum := start + 500
			boundary := 0
			for i := end - 1; i >= minimum; i-- {
				if source[i] == '\n' && i > start && source[i-1] == '\n' {
					boundary = i + 1
					break
				}
			}
			if boundary == 0 {
				for i := end - 1; i >= minimum; i-- {
					if (source[i] == '.' || source[i] == '!' || source[i] == '?' || source[i] == '。') && i+1 < len(source) && unicode.IsSpace(source[i+1]) {
						boundary = i + 1
						break
					}
				}
			}
			if boundary != 0 {
				end = boundary
			}
		}
		slice := string(source[start:end])
		if strings.TrimSpace(slice) != "" {
			out = append(out, Chunk{Text: slice, Start: start, End: end})
		}
		if end == len(source) {
			break
		}
		start = end - 100
	}
	return out
}
