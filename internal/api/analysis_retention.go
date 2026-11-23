package api

import (
	"context"
	"log"
	"time"
)

func startAnalysisHistoryCleanup(purge func(context.Context) error) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for ctx.Err() == nil {
			if err := purge(ctx); err != nil && ctx.Err() == nil {
				log.Printf("analysis history cleanup failed, retrying on the next sweep: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
