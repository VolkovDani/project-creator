package rag

import (
	"context"
	"testing"
	"time"
)

func waitForWaiters(t *testing.T, s *Service, interactive, background int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.inferenceMu.Lock()
		ok := s.inferenceInteractive == interactive && s.inferenceBackground == background
		s.inferenceMu.Unlock()
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scheduler waiters did not arrive")
}

func TestInteractiveInferencePrecedesBackground(t *testing.T) {
	s := New(nil, nil)
	started, release, order := make(chan struct{}), make(chan struct{}), make(chan string, 2)
	go func() {
		_ = s.withInference(context.Background(), true, func() error { close(started); <-release; return nil })
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("active=%v interactive=%d background=%d burst=%d", s.inferenceActive, s.inferenceInteractive, s.inferenceBackground, s.interactiveBurst)
	}
	go func() {
		_ = s.withInference(context.Background(), false, func() error { order <- "background"; return nil })
	}()
	waitForWaiters(t, s, 1, 1)
	go func() {
		_ = s.withInference(context.Background(), true, func() error { order <- "interactive"; return nil })
	}()
	waitForWaiters(t, s, 2, 1)
	close(release)
	select {
	case got := <-order:
		if got != "interactive" {
			t.Fatalf("first inference=%s, want interactive", got)
		}
	case <-time.After(time.Second):
		t.Fatal("inference scheduler stalled")
	}
}

func TestInteractiveWaitersAreBoundedAndCancellable(t *testing.T) {
	s := New(nil, nil)
	started, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = s.withInference(context.Background(), false, func() error { close(started); <-release; return nil })
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.withInference(ctx, true, func() error { return nil }); err != context.Canceled {
		t.Fatalf("cancelled inference error=%v, want context cancellation", err)
	}
	for i := 0; i < maxInteractiveWaiters; i++ {
		go func() { _ = s.withInference(context.Background(), true, func() error { return nil }) }()
	}
	waitForWaiters(t, s, maxInteractiveWaiters, 1)
	if err := s.withInference(context.Background(), true, func() error { return nil }); err != ErrBusy {
		t.Fatalf("overflow error=%v, want ErrBusy", err)
	}
	close(release)
}

func TestCancelledInferenceDoesNotStartWhenIdle(t *testing.T) {
	s := New(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := s.withInference(ctx, true, func() error { called = true; return nil })
	if err != context.Canceled || called {
		t.Fatalf("cancelled admission started inference: called=%v err=%v", called, err)
	}
}

func TestBackgroundGetsTurnAfterInteractiveBurst(t *testing.T) {
	s := New(nil, nil)
	s.inferenceActive = true
	s.interactiveBurst = maxInteractiveBurst
	order := make(chan string, 2)
	go func() {
		_ = s.withInference(context.Background(), true, func() error { order <- "search"; return nil })
	}()
	go func() {
		_ = s.withInference(context.Background(), false, func() error { order <- "index"; return nil })
	}()
	waitForWaiters(t, s, 1, 1)
	s.inferenceMu.Lock()
	s.inferenceActive = false
	close(s.inferenceChanged)
	s.inferenceChanged = make(chan struct{})
	s.inferenceMu.Unlock()
	select {
	case first := <-order:
		if first != "index" {
			t.Fatalf("background starved: %s", first)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler stalled")
	}
}
