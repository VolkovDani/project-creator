package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const layaModelTag = "laya:322m-multilingual-bf16"
const layaPinnedDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func layaPairFixture() Pair {
	return Pair{Left: Source{ID: "left", Version: 2, Text: "Payment must use a card."}, Right: Source{ID: "right", Version: 4, Text: "Payments may use cash."}}
}

func layaTags(w http.ResponseWriter, digest string) {
	fmt.Fprintf(w, `{"models":[{"name":"%s","digest":"sha256:%s"}]}`, layaModelTag, digest)
}

func layaAnswer(w http.ResponseWriter, decision string, scores map[string]float64) {
	json.NewEncoder(w).Encode(map[string]any{
		"model": layaModelTag,
		"answers": map[string]any{"decision": map[string]any{
			"type": "choice", "choice": decision, "probabilities": scores, "confidence": 0.5,
		}},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
	})
}

func TestLayaUsesPinnedLocalSystemOneAndPreservesProvenance(t *testing.T) {
	tags := 0
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			tags++
			layaTags(w, layaPinnedDigest)
		case "/v1/systemone":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			layaAnswer(w, "related", map[string]float64{"related": .7, "unrelated": .2, "uncertain": .1})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	classifier, err := NewLaya(server.URL, layaModelTag, layaPinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	pair := layaPairFixture()
	pair.Left.Text = strings.Repeat("x", 180)
	pair.Right.Text = strings.Repeat("y", 180)
	got, err := classifier.Classify(context.Background(), pair)
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != "related" || got.Scores["related"] != .7 || tags != 2 {
		t.Fatalf("got %+v, tags=%d", got, tags)
	}
	if request["model"] != layaModelTag {
		t.Fatalf("model not pinned: %#v", request)
	}
	if request["keep_alive"] != "5m" {
		t.Fatalf("keep_alive not pinned: %#v", request["keep_alive"])
	}
	state, ok := request["state"].(map[string]any)
	if !ok || state["left"] == nil || state["right"] == nil {
		t.Fatalf("pair not sent as structured state: %#v", request["state"])
	}
	questions := request["questions"].(map[string]any)
	decision := questions["decision"].(map[string]any)
	if decision["type"] != "choice" {
		t.Fatalf("unexpected question: %#v", decision)
	}
	criteria := decision["criteria"].(map[string]any)
	if len(criteria) != 3 || criteria["related"] == nil || criteria["unrelated"] == nil || criteria["uncertain"] == nil {
		t.Fatalf("incomplete decision choices: %#v", criteria)
	}
	var provenance map[string]any
	if err := json.Unmarshal([]byte(classifier.Provenance()), &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance["adapter"] != "ollama-systemone-v1" || provenance["model"] != layaModelTag || provenance["digest"] != layaPinnedDigest {
		t.Fatalf("provenance does not pin adapter/model/digest: %#v", provenance)
	}
	parameters := provenance["parameters"].(map[string]any)
	if parameters["question"] == nil || parameters["maxStateBytes"] != float64(704) || parameters["maxModelTokens"] != float64(1024) || parameters["headTokens"] != float64(256) || parameters["keepAlive"] != "5m" || provenance["responseSchema"] == nil || classifier.Profile() == "" {
		t.Fatalf("incomplete provenance: %#v", provenance)
	}
}

func TestLayaRejectsInvalidOutputsBeforeReturningClassification(t *testing.T) {
	for _, scenario := range []string{"unknown-choice", "missing-score", "extra-score", "nan-score", "out-of-range", "wrong-type", "malformed-envelope", "oversize", "digest-change", "remote-model", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			tags := 0
			followedRedirect := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect-target" {
					followedRedirect = true
					layaAnswer(w, "related", map[string]float64{"related": .7, "unrelated": .2, "uncertain": .1})
					return
				}
				if r.URL.Path == "/api/tags" {
					tags++
					if scenario == "digest-change" && tags > 1 {
						layaTags(w, strings.Repeat("d", 64))
					} else if scenario == "remote-model" {
						fmt.Fprintf(w, `{"models":[{"name":"%s","digest":"sha256:%s","remote_host":"cloud"}]}`, layaModelTag, layaPinnedDigest)
					} else {
						layaTags(w, layaPinnedDigest)
					}
					return
				}
				if scenario == "oversize" {
					w.Write([]byte(strings.Repeat("x", maxResponse+1)))
					return
				}
				if scenario == "redirect" {
					http.Redirect(w, r, "/redirect-target", http.StatusFound)
					return
				}
				if scenario == "malformed-envelope" {
					fmt.Fprintf(w, `{"model":%q,"answers":[]}`, layaModelTag)
					return
				}
				scores := map[string]float64{"related": .7, "unrelated": .2, "uncertain": .1}
				choice := "related"
				switch scenario {
				case "unknown-choice":
					choice = "maybe"
				case "missing-score":
					delete(scores, "uncertain")
				case "extra-score":
					scores["other"] = .01
				case "nan-score":
					scores["related"] = math.NaN()
				case "out-of-range":
					scores["related"] = 1.1
				case "wrong-type":
					json.NewEncoder(w).Encode(map[string]any{"model": layaModelTag, "answers": map[string]any{"decision": map[string]any{"type": "score", "score": 1}}})
					return
				}
				layaAnswer(w, choice, scores)
			}))
			defer server.Close()
			classifier, err := NewLaya(server.URL, layaModelTag, layaPinnedDigest)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = classifier.Classify(context.Background(), layaPairFixture()); err == nil {
				t.Fatal("accepted invalid Laya response or model identity")
			}
			if followedRedirect {
				t.Fatal("followed a redirect")
			}
		})
	}
}

func TestLayaRejectsRemoteURLsAndLongInputs(t *testing.T) {
	for _, endpoint := range []string{"https://localhost:11434", "http://example.com", "http://user:secret@localhost:11434", "http://localhost:11434/path", "http://localhost:11434?x=y"} {
		if _, err := NewLaya(endpoint, "laya:multilingual", layaPinnedDigest); err == nil {
			t.Errorf("accepted endpoint %q", endpoint)
		}
	}
	classifier, err := NewLaya("http://ollama:11434", layaModelTag, layaPinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	pair := layaPairFixture()
	pair.Left.Text = strings.Repeat("я", 400)
	if _, err := classifier.Classify(context.Background(), pair); !errors.Is(err, ErrClassificationInputTooLong) {
		t.Fatalf("long input not rejected before request: %v", err)
	}
	if _, err := NewLaya("http://localhost:11434", "laya:multilingual", layaPinnedDigest); err == nil {
		t.Fatal("accepted unpinned alias instead of the multilingual BF16 tag")
	}
	if _, err := NewLaya("http://localhost:11434", layaModelTag, "short"); err == nil {
		t.Fatal("accepted missing digest pin")
	}
}

func TestLayaPropagatesCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			layaTags(w, layaPinnedDigest)
			return
		}
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	classifier, err := NewLaya(server.URL, layaModelTag, layaPinnedDigest)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, classifyErr := classifier.Classify(ctx, layaPairFixture())
		done <- classifyErr
	}()
	<-entered
	cancel()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestLayaRequiresNumericScoresConfidenceAndUsage(t *testing.T) {
	for _, invalid := range []string{"null-score", "missing-confidence", "null-confidence", "missing-usage"} {
		t.Run(invalid, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					layaTags(w, layaPinnedDigest)
					return
				}
				scores := map[string]any{"related": .5, "unrelated": .5, "uncertain": 0.0}
				answer := map[string]any{"type": "choice", "choice": "related", "probabilities": scores, "confidence": .5}
				result := map[string]any{"model": layaModelTag, "answers": map[string]any{"decision": answer}, "usage": map[string]int{"input_tokens": 12, "output_tokens": 0}}
				switch invalid {
				case "null-score":
					scores["uncertain"] = nil
				case "missing-confidence":
					delete(answer, "confidence")
				case "null-confidence":
					answer["confidence"] = nil
				case "missing-usage":
					delete(result, "usage")
				}
				json.NewEncoder(w).Encode(result)
			}))
			defer server.Close()
			classifier, err := NewLaya(server.URL, layaModelTag, layaPinnedDigest)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = classifier.Classify(context.Background(), layaPairFixture()); err == nil {
				t.Fatal("accepted response outside required numeric schema")
			}
		})
	}
}
