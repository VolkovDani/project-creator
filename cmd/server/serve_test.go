package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeHTTPStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listening := make(chan struct{})
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler(), BaseContext: func(net.Listener) context.Context {
		close(listening)
		return context.Background()
	}}
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, server) }()
	select {
	case <-listening:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP server did not listen")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server ignored shutdown")
	}
}

func TestServeHTTPReturnsListenFailure(t *testing.T) {
	if err := serveHTTP(context.Background(), &http.Server{Addr: "127.0.0.1:-1"}); err == nil {
		t.Fatal("listen failure was hidden")
	}
}

func TestServeHTTPClosesActiveSSEOnSignalContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listening := make(chan string, 1)
	handlerDone := make(chan struct{})
	server := &http.Server{Addr: "127.0.0.1:0", BaseContext: func(listener net.Listener) context.Context { listening <- listener.Addr().String(); return ctx }, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(handlerDone)
	})}
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, server) }()
	var addr string
	select {
	case addr = <-listening:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not listen")
	}
	response, err := http.Get("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("normal signal with an SSE client returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SSE blocked graceful shutdown")
	}
	select {
	case <-handlerDone:
	default:
		t.Fatal("SSE handler did not observe signal cancellation")
	}
}
