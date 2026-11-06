package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func npuFindingJSON() string {
	return `{"leftId":"left","rightId":"right","leftVersion":1,"rightVersion":1,"verdict":"collision","explanation":"В одном сценарии разрешены взаимоисключающие способы оплаты."}`
}

func npuServer(t *testing.T, h http.HandlerFunc) *OpenVINO {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	generator, err := NewOpenVINO("/run/npu/runtime.sock", pinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise real HTTP parsing and deadlines even where AF_UNIX bind is blocked.
	generator.client.Transport = &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	return generator
}

func TestOpenVINOUnixSocketTransport(t *testing.T) {
	dir, err := os.MkdirTemp("", "pc-npu-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "runtime.sock")
	listener, err := net.Listen("unix", socket)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("host sandbox blocks AF_UNIX bind; HTTP protocol covered separately")
	}
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"profile":"` + pinnedDigest + `","device":"NPU"}`))
	})}
	go server.Serve(listener)
	defer server.Close()
	generator, err := NewOpenVINO(socket, pinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := generator.verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOpenVINOComparisonUsesPinnedNPUAndUntrustedPair(t *testing.T) {
	calls := 0
	generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			calls++
			json.NewEncoder(w).Encode(map[string]string{"profile": pinnedDigest, "device": "NPU"})
			return
		}
		if r.Method != "POST" || r.URL.Path != "/compare" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Profile string         `json:"profile"`
			System  string         `json:"system"`
			Pair    Pair           `json:"pair"`
			Schema  map[string]any `json:"schema"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Profile != pinnedDigest || !strings.Contains(request.System, "untrusted") || request.Pair.Left.ID != "left" || request.Schema["additionalProperties"] != false {
			t.Errorf("bad request: %+v", request)
		}
		var finding Finding
		json.Unmarshal([]byte(npuFindingJSON()), &finding)
		json.NewEncoder(w).Encode(map[string]any{"profile": pinnedDigest, "device": "NPU", "finding": finding})
	})
	result, err := generator.Compare(context.Background(), pairFixture())
	if err != nil || result.Verdict != "collision" || calls != 2 {
		t.Fatalf("result=%+v err=%v health=%d", result, err, calls)
	}
	other, _ := NewOpenVINO("/run/npu/runtime.sock", strings.Repeat("b", 64))
	if generator.Profile() == other.Profile() || generator.Profile() == pinnedDigest {
		t.Fatal("profile does not include protocol/runtime identity")
	}
}

func TestOpenVINODuplicatePreservesSourceReferences(t *testing.T) {
	generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{"profile": pinnedDigest, "device": "NPU"})
			return
		}
		var finding Finding
		json.Unmarshal([]byte(strings.Replace(npuFindingJSON(), `"collision"`, `"duplicate"`, 1)), &finding)
		json.NewEncoder(w).Encode(map[string]any{"profile": pinnedDigest, "device": "NPU", "finding": finding})
	})
	pair := Pair{Left: Source{ID: "original-left", Version: 7, Text: "Z"}, Right: Source{ID: "original-right", Version: 11, Text: "A"}}
	finding, err := generator.Compare(context.Background(), pair)
	if err != nil || finding.Verdict != "duplicate" || finding.LeftID != pair.Left.ID || finding.LeftVersion != 7 || finding.RightID != pair.Right.ID || finding.RightVersion != 11 {
		t.Fatalf("duplicate references lost through normalization: %+v %v", finding, err)
	}
}

func TestOpenVINOAliasReferencesRoundTripAndRejectUnknownSources(t *testing.T) {
	pair := Pair{
		Left: Source{ID: "348a9ac1d8a3cce4f2384ad79a3f070c", Version: 7,
			Text: "При любом заказе оплата разрешена только наличными. Другие способы запрещены."},
		Right: Source{ID: "f10d2fd077690e2b42821b0f7c2b178a", Version: 19,
			Text: "При любом заказе оплата разрешена только банковской картой. Другие способы запрещены."},
	}
	for _, input := range []struct {
		name string
		pair Pair
	}{{"original", pair}, {"reversed", Pair{Left: pair.Right, Right: pair.Left}}} {
		t.Run(input.name, func(t *testing.T) {
			for _, response := range []struct {
				name                      string
				leftID, rightID           string
				leftVersion, rightVersion int
				valid                     bool
			}{
				{"aliases", "left", "right", 1, 1, true},
				{"foreign_alias", "unknown", "right", 1, 1, false},
				{"swapped_aliases", "right", "left", 1, 1, false},
				{"real_references", input.pair.Left.ID, input.pair.Right.ID, input.pair.Left.Version, input.pair.Right.Version, false},
				{"wrong_left_version", "left", "right", 2, 1, false},
				{"wrong_right_version", "left", "right", 1, 2, false},
			} {
				t.Run(response.name, func(t *testing.T) {
					generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/health" {
							json.NewEncoder(w).Encode(map[string]string{"profile": pinnedDigest, "device": "NPU"})
							return
						}
						var request struct {
							Pair Pair `json:"pair"`
						}
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						want := Pair{
							Left:  Source{ID: "left", Version: 1, Text: input.pair.Left.Text},
							Right: Source{ID: "right", Version: 1, Text: input.pair.Right.Text},
						}
						if want.Right.Text < want.Left.Text {
							want.Left.Text, want.Right.Text = want.Right.Text, want.Left.Text
						}
						if request.Pair != want {
							t.Errorf("model received %+v; want normalized references and unchanged texts %+v", request.Pair, want)
						}
						finding := Finding{LeftID: response.leftID, RightID: response.rightID,
							LeftVersion: response.leftVersion, RightVersion: response.rightVersion,
							Verdict: "collision", Explanation: "В одном сценарии требуются взаимоисключающие способы оплаты."}
						json.NewEncoder(w).Encode(map[string]any{"profile": pinnedDigest, "device": "NPU", "finding": finding})
					})
					finding, err := generator.Compare(context.Background(), input.pair)
					if !response.valid {
						if err == nil {
							t.Fatalf("accepted invalid model references: %+v", finding)
						}
						return
					}
					if err != nil || !validFinding(input.pair, finding) || finding.Verdict != "collision" {
						t.Fatalf("references did not round trip: finding=%+v err=%v", finding, err)
					}
				})
			}
		})
	}
}

func TestOpenVINOInputOrderDoesNotChangeModelPair(t *testing.T) {
	first := Source{ID: "a98da36e7bf38c6299ccd09648660d83", Version: 7,
		Text: "Для заказов стоимостью до 1000 рублей включительно доставка бесплатна."}
	second := Source{ID: "abd201e6bdf2ea03fd9fa7d29f3c3ebd", Version: 19,
		Text: "Для заказов стоимостью больше 1000 рублей доставка стоит 100 рублей."}
	var received []Pair
	generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{"profile": pinnedDigest, "device": "NPU"})
			return
		}
		var request struct {
			Pair Pair `json:"pair"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		received = append(received, request.Pair)
		finding := Finding{LeftID: "left", RightID: "right", LeftVersion: 1, RightVersion: 1,
			Verdict: "compatible", Explanation: "Условия стоимости заказа не пересекаются."}
		json.NewEncoder(w).Encode(map[string]any{"profile": pinnedDigest, "device": "NPU", "finding": finding})
	})
	for _, pair := range []Pair{{Left: first, Right: second}, {Left: second, Right: first}} {
		finding, err := generator.Compare(context.Background(), pair)
		if err != nil || !validFinding(pair, finding) || finding.Verdict != "compatible" {
			t.Fatalf("original references not restored: finding=%+v err=%v", finding, err)
		}
	}
	want := Pair{
		Left:  Source{ID: "left", Version: 1, Text: second.Text},
		Right: Source{ID: "right", Version: 1, Text: first.Text},
	}
	if len(received) != 2 || received[0] != want || received[1] != want {
		t.Fatalf("input order changed the model pair: got=%+v want=%+v", received, want)
	}
}

func TestOpenVINORejectsConfigurationAndBadResponses(t *testing.T) {
	for _, cfg := range [][2]string{{"relative.sock", pinnedDigest}, {"/tmp/x", "sha256:" + pinnedDigest}, {"/tmp/x", strings.Repeat("A", 64)}, {"/tmp/x", ""}} {
		if _, err := NewOpenVINO(cfg[0], cfg[1]); err == nil {
			t.Errorf("accepted invalid config %v", cfg)
		}
	}
	for _, tc := range []struct{ name, health, response string }{
		{"cpu", `{"profile":"` + pinnedDigest + `","device":"CPU"}`, ""},
		{"profile", `{"profile":"` + strings.Repeat("b", 64) + `","device":"NPU"}`, ""},
		{"response cpu", "", `{"profile":"` + pinnedDigest + `","device":"CPU","finding":` + npuFindingJSON() + `}`},
		{"response profile", "", `{"profile":"` + strings.Repeat("b", 64) + `","device":"NPU","finding":` + npuFindingJSON() + `}`},
		{"malformed", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":{}}`},
		{"foreign source", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + strings.Replace(npuFindingJSON(), "left\"", "foreign\"", 1) + `}`},
		{"extra field", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + npuFindingJSON() + `,"tool_calls":[]}`},
		{"trailing", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + npuFindingJSON() + `} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					if tc.health != "" {
						w.Write([]byte(tc.health))
					} else {
						w.Write([]byte(`{"profile":"` + pinnedDigest + `","device":"NPU"}`))
					}
					return
				}
				w.Write([]byte(tc.response))
			})
			if _, err := generator.Compare(context.Background(), pairFixture()); err == nil {
				t.Fatal("accepted invalid runtime response")
			}
		})
	}
}

func TestOpenVINORejectsInvalidSourceBeforeRuntime(t *testing.T) {
	generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached runtime")
		w.WriteHeader(http.StatusInternalServerError)
	})
	pair := pairFixture()
	pair.Right.ID = pair.Left.ID
	if _, err := generator.Compare(context.Background(), pair); err == nil {
		t.Fatal("accepted repeated source")
	}
	pair = pairFixture()
	pair.Left.Text = strings.Repeat("я", 2001)
	if _, err := generator.Compare(context.Background(), pair); err == nil {
		t.Fatal("accepted oversized source")
	}
}

func TestOpenVINODriftAndCancellation(t *testing.T) {
	calls := 0
	generator := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			calls++
			profile := pinnedDigest
			if calls > 1 {
				profile = strings.Repeat("b", 64)
			}
			json.NewEncoder(w).Encode(map[string]string{"profile": profile, "device": "NPU"})
			return
		}
		w.Write([]byte(`{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + npuFindingJSON() + `}`))
	})
	if _, err := generator.Compare(context.Background(), pairFixture()); err == nil {
		t.Fatal("accepted runtime profile drift")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if _, err := generator.Compare(ctx, pairFixture()); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestOpenVINOReportsSafeRuntimeFailureCodes(t *testing.T) {
	for _, code := range []string{"NPU_UNAVAILABLE", "NPU_RECOVERY_FAILED", "NPU_WORKER_EXIT", "TIMEOUT", "INVALID_OUTPUT", "PROFILE_CHANGED", "private unknown error"} {
		t.Run(code, func(t *testing.T) {
			o := npuServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "error": "private model text"})
			})
			_, got := checkedCompare(context.Background(), o, Pair{Left: Source{ID: "left", Version: 1, Text: "First"}, Right: Source{ID: "right", Version: 1, Text: "Second"}})
			want := code
			if code == "private unknown error" {
				want = "GENERATION_FAILED"
			}
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}
