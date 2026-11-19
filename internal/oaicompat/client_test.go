package oaicompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRejectedRequestsStayAtConfiguredEndpoint(t *testing.T) {
	var forwarded atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded.Store(true) }))
	defer other.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/test" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Error("incorrect route or credentials")
		}
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer s.Close()
	c, err := New(s.URL+"/v1/", "model", "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Post(context.Background(), "/test", map[string]string{"input": "private"}, new(any)); err == nil || forwarded.Load() || strings.Contains(err.Error(), "secret-token") {
		t.Fatal("redirect or credential disclosure")
	}
}

func TestEndpointAndModelIdentity(t *testing.T) {
	for _, base := range []string{"", "//host/v1", "https://user:secret@host/v1", "https://host/v1?key=secret", "https://host/v1#fragment", "file:///tmp/models"} {
		if _, err := New(base, "model", ""); err == nil {
			t.Errorf("accepted invalid endpoint")
		}
	}
	a, _ := New("https://host/v1", "a", "secret")
	b, _ := New("https://host/v1/", "a", "rotated")
	c, _ := New("https://host/v1", "b", "secret")
	d, _ := New("https://other/v1", "a", "secret")
	if a.Identity() != b.Identity() || a.Identity() == c.Identity() || a.Identity() == d.Identity() {
		t.Fatal("incorrect model identity")
	}
}

func TestAuthFailureIsSanitizedAndNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, r.Header.Get("Authorization")+" private provider diagnostics", http.StatusUnauthorized)
	}))
	defer server.Close()
	c, err := New(server.URL, "model", "private-key")
	if err != nil {
		t.Fatal(err)
	}
	err = c.Post(context.Background(), "/embeddings", map[string]string{"input": "private input"}, new(any))
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "private") || calls.Load() != 1 {
		t.Fatalf("incorrect authentication error or retry count: %v, %d", err, calls.Load())
	}
}
