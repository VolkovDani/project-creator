package analysis

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/classification"
	"github.com/VolkovDani/project-creator/internal/explanation"
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
	checkNPUComparisonFixtures(t, generator)
}

// Opt in with a loopback manager endpoint and its immutable OpenVINO model ID.
func TestManagedOpenVINORealNPUSmoke(t *testing.T) {
	if os.Getenv("MANAGED_NPU_REAL_SMOKE") != "1" {
		t.Skip("explicit MANAGED_NPU_REAL_SMOKE=1 and a prepared manager required")
	}
	model := os.Getenv("MANAGED_NPU_MODEL")
	if !managedNPUModelID.MatchString(model) {
		t.Fatal("select the full managed NPU profile ID")
	}
	generator, err := NewOpenAICompatible(os.Getenv("MANAGED_NPU_URL"), model, "")
	if err != nil {
		t.Fatal(err)
	}
	checkNPUComparisonFixtures(t, generator)
}

func TestManagedNPURealClassificationExplanation(t *testing.T) {
	if os.Getenv("MANAGED_NPU_REAL_SMOKE") != "1" {
		t.Skip("explicit MANAGED_NPU_REAL_SMOKE=1 and a prepared manager required")
	}
	g, err := NewOpenAICompatible(os.Getenv("MANAGED_NPU_URL"), os.Getenv("MANAGED_NPU_MODEL"), "")
	if err != nil {
		t.Fatal(err)
	}
	const text = "Как бухгалтер, я хочу выгружать реестр платежей в CSV, чтобы сверять поступления."
	source := explanation.Source{Text: text, Suggestion: &classification.Entry{
		Source: "model", Status: "suggested", Semantic: "functional_requirement", Representation: "user_story",
	}}
	target := explanation.Target{Semantic: "functional_requirement", Representation: "user_story", Locale: "ru"}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	start := time.Now()
	got, err := g.ExplainClassification(ctx, source, target)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("assessment returned in %s: %+v", time.Since(start).Round(time.Millisecond), got)
	if got.Meaning.Verdict != "fits" || got.Form.Verdict != "fits" {
		t.Fatal("the explicit behavior and user story must both fit")
	}
	if len(got.Meaning.Criteria) != 1 || len(got.Form.Criteria) != 3 {
		t.Fatal("expected system behavior, role, action and value")
	}
	for _, criterion := range append(got.Meaning.Criteria, got.Form.Criteria...) {
		if criterion.Status != "present" || criterion.Evidence != text {
			t.Fatalf("explicit criterion must cite the original text: %+v", criterion)
		}
	}
	if len(got.Questions) != 0 {
		t.Fatal("a complete user story should have no required gaps")
	}
}

func checkNPUComparisonFixtures(t *testing.T, generator Generator) {
	t.Helper()
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

// Regression: short IDs and unequal versions hid verdict changes seen by setup.
// Keep the installation source IDs, equal versions and both input orientations.
// The additional pairs check that quality is not limited to the setup fixtures.
func TestOpenVINORealNPUInstallationPairs(t *testing.T) {
	if os.Getenv("NPU_REAL_SMOKE") != "1" {
		t.Skip("explicit NPU_REAL_SMOKE=1 and a prepared local runtime required")
	}
	generator, err := NewOpenVINO(os.Getenv("PROJECT_CREATOR_NPU_SOCKET"), os.Getenv("PROJECT_CREATOR_NPU_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, leftID, left, rightID, right, verdict string
	}{
		{
			"installation_collision",
			"348a9ac1d8a3cce4f2384ad79a3f070c", "При любом заказе оплата разрешена только наличными. Другие способы запрещены.",
			"f10d2fd077690e2b42821b0f7c2b178a", "При любом заказе оплата разрешена только банковской картой. Другие способы запрещены.",
			"collision",
		},
		{
			"installation_compatible",
			"a98da36e7bf38c6299ccd09648660d83", "Пользователь может выбрать оплату наличными вместо банковской карты.",
			"abd201e6bdf2ea03fd9fa7d29f3c3ebd", "Пользователь может оплатить заказ банковской картой.",
			"compatible",
		},
		{
			"installation_insufficient",
			"0b3084dba645d11b10d163712c047676", "Для некоторых заказов обработка может занимать больше времени.",
			"24e07cee129868cde3ab0fd781c62d9b", "Заказ должен быть обработан быстро.",
			"insufficient",
		},
		{
			"undefined_measurement",
			"b64a8a602a5e47cfbb04d92fdd3ab208", "Архив отчётов должен занимать мало места на диске.",
			"cd23bf864adb481ba027896815f44229", "Архив отчётов должен сохранять все исходные изображения без потери качества.",
			"insufficient",
		},
		{
			"measurable_compatible",
			"c9f1a6d19af94304b56b4f2f84aa6d36", "Для заказов стоимостью до 1000 рублей включительно доставка бесплатна.",
			"d27f98a65c3d4df39ef537758254949b", "Для заказов стоимостью больше 1000 рублей доставка стоит 100 рублей.",
			"compatible",
		},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			original := Pair{
				Left:  Source{ID: example.leftID, Version: 1, Text: example.left},
				Right: Source{ID: example.rightID, Version: 1, Text: example.right},
			}
			for _, orientation := range []struct {
				name string
				pair Pair
			}{{"original", original}, {"reversed", Pair{Left: original.Right, Right: original.Left}}} {
				t.Run(orientation.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					defer cancel()
					start := time.Now()
					finding, err := generator.Compare(ctx, orientation.pair)
					if err != nil {
						t.Fatal(err)
					}
					data, err := json.Marshal(map[string]any{"case": example.name, "orientation": orientation.name, "generator_profile": generator.Profile(), "seconds": time.Since(start).Seconds(), "finding": finding})
					if err != nil {
						t.Fatal(err)
					}
					t.Log(string(data))
					if !validFinding(orientation.pair, finding) || finding.Verdict != example.verdict {
						t.Errorf("quality check: expected valid %s finding, got %+v", example.verdict, finding)
					}
				})
			}
		})
	}
}
