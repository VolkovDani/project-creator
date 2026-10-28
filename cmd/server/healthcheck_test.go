package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckVerifiesAuthorizedDatabaseQuery(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		ready  bool
	}{
		{`{"data":{"projects":[]}}`, 200, true},
		{`{"errors":[{"message":"database unavailable"}]}`, 200, false},
		{`{"data":{"projects":null}}`, 200, false},
		{`{"data":{"projects":[]},"errors":[{}]}`, 200, false},
		{`Unauthorized`, 401, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/graphql" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private-token" {
				t.Error("invalid healthcheck request")
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		err := healthcheck(context.Background(), server.URL+"/graphql", "private-token")
		if (err == nil) != tc.ready {
			t.Errorf("response %s ready=%v err=%v", tc.body, tc.ready, err)
		}
		server.Close()
	}
}
