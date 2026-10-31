package main

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// A bounded HTTP shutdown releases SSE connections before the caller closes
// background workers and their database. Listen errors return to that caller so
// resource cleanup happens before main can use log.Fatal.
func serveHTTP(ctx context.Context, server *http.Server) error {
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		listenErr := <-done
		if !errors.Is(listenErr, http.ErrServerClosed) {
			return listenErr
		}
		return err
	}
}
