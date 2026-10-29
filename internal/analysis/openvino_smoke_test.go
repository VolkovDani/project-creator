package analysis

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Explicit opt-in hardware/quality probe. Never runs in ordinary CI and does not
// equate a successful HTTP response with a verified expected finding.
func TestOpenVINORealNPUSmoke(t *testing.T) {
	if os.Getenv("NPU_REAL_SMOKE") != "1" {
		t.Skip("explicit NPU_REAL_SMOKE=1 and a prepared local runtime required")
	}
	generator, err := NewOpenVINO(os.Getenv("PROJECT_CREATOR_NPU_SOCKET"), os.Getenv("PROJECT_CREATOR_NPU_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, left, right, verdict string }{
		{"collision", "При любом заказе оплата разрешена только банковской картой. Другие способы запрещены.", "При любом заказе оплата разрешена только наличными. Другие способы запрещены.", "collision"},
		{"compatible", "Пользователь может оплатить заказ банковской картой.", "Пользователь может выбрать оплату наличными вместо банковской карты.", "compatible"},
		{"insufficient", "Заказ должен быть обработан быстро.", "Для некоторых заказов обработка может занимать больше времени.", "insufficient"},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			pair := Pair{Left: Source{ID: "left", Version: 1, Text: example.left}, Right: Source{ID: "right", Version: 2, Text: example.right}}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			start := time.Now()
			finding, err := generator.Compare(ctx, pair)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(map[string]any{"case": example.name, "generator_profile": generator.Profile(), "seconds": time.Since(start).Seconds(), "finding": finding})
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(data))
			if finding.Verdict != example.verdict {
				t.Errorf("quality check: expected %s, got %s; inspect sources and explanation", example.verdict, finding.Verdict)
			}
		})
	}
}

// Regression: the prompt-only NPU pipeline repeated the explanation key for
// application-sized IDs with equal versions, so the runtime rejected its JSON.
func TestOpenVINORealNPUApplicationIDs(t *testing.T) {
	if os.Getenv("NPU_REAL_SMOKE") != "1" {
		t.Skip("explicit NPU_REAL_SMOKE=1 and a prepared local runtime required")
	}
	generator, err := NewOpenVINO(os.Getenv("PROJECT_CREATOR_NPU_SOCKET"), os.Getenv("PROJECT_CREATOR_NPU_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	pair := Pair{
		Left: Source{ID: "30c956a76badd4a9871754a8efd98a97", Version: 1,
			Text: "При любом заказе оплата разрешена только банковской картой. Другие способы запрещены."},
		Right: Source{ID: "c1c4ede0fb916329925ca2f3e8dbb47a", Version: 1,
			Text: "При любом заказе оплата разрешена только наличными. Другие способы запрещены."},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	start := time.Now()
	finding, err := generator.Compare(ctx, pair)
	if err != nil {
		t.Fatal(err)
	}
	if !validFinding(pair, finding) || finding.Verdict != "collision" {
		t.Fatalf("invalid collision finding: %+v", finding)
	}
	t.Logf("application IDs: %.2fs, profile=%s, finding=%+v", time.Since(start).Seconds(), generator.Profile(), finding)
}
