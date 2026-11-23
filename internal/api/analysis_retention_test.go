package api

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestAnalysisHistoryCleanupRetriesAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		stop := startAnalysisHistoryCleanup(func(context.Context) error {
			attempts++
			if attempts == 1 {
				return errors.New("database unavailable")
			}
			return nil
		})
		synctest.Wait()
		if attempts != 1 {
			t.Fatalf("expected one startup sweep, got %d", attempts)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		if attempts != 2 {
			t.Fatalf("expected an hourly retry, got %d sweeps", attempts)
		}
		stop()
		time.Sleep(time.Hour)
		synctest.Wait()
		if attempts != 2 {
			t.Fatalf("cleanup ran after shutdown: %d sweeps", attempts)
		}
	})
}

func TestAnalysisHistoryCleanupCancelsAndJoinsActiveSweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		finished := false
		stop := startAnalysisHistoryCleanup(func(ctx context.Context) error {
			<-ctx.Done()
			finished = true
			return ctx.Err()
		})
		synctest.Wait()
		stop()
		if !finished {
			t.Fatal("shutdown returned before the sweep released its database")
		}
	})
}

func TestConfiguredAnalysisHistoryPolicy(t *testing.T) {
	path := testDatabase(t)
	for _, tc := range []struct{ configured, want int }{{0, 30}, {7, 7}} {
		s, err := NewConfigured(path, map[string]string{"token-a": "a"}, Config{AnalysisHistoryRetentionDays: tc.configured})
		if err != nil {
			t.Fatal(err)
		}
		got := s.analysis.RetentionDays()
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("configured %d, got %d; want %d days", tc.configured, got, tc.want)
		}
	}
}
