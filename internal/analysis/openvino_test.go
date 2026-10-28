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
		json.Unmarshal([]byte(findingJSON()), &finding)
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

func TestOpenVINORejectsConfigurationAndBadResponses(t *testing.T) {
	for _, cfg := range [][2]string{{"relative.sock", pinnedDigest}, {"/tmp/x", "sha256:" + pinnedDigest}, {"/tmp/x", strings.Repeat("A", 64)}, {"/tmp/x", ""}} {
		if _, err := NewOpenVINO(cfg[0], cfg[1]); err == nil {
			t.Errorf("accepted invalid config %v", cfg)
		}
	}
	for _, tc := range []struct{ name, health, response string }{
		{"cpu", `{"profile":"` + pinnedDigest + `","device":"CPU"}`, ""},
		{"profile", `{"profile":"` + strings.Repeat("b", 64) + `","device":"NPU"}`, ""},
		{"response cpu", "", `{"profile":"` + pinnedDigest + `","device":"CPU","finding":` + findingJSON() + `}`},
		{"response profile", "", `{"profile":"` + strings.Repeat("b", 64) + `","device":"NPU","finding":` + findingJSON() + `}`},
		{"malformed", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":{}}`},
		{"foreign source", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + strings.Replace(findingJSON(), "left\"", "foreign\"", 1) + `}`},
		{"extra field", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + findingJSON() + `,"tool_calls":[]}`},
		{"trailing", "", `{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + findingJSON() + `} {}`},
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
		w.Write([]byte(`{"profile":"` + pinnedDigest + `","device":"NPU","finding":` + findingJSON() + `}`))
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
