// Package jobs runs periodic background maintenance.
package jobs

import (
	"context"
	"time"
)

// Every runs job once immediately and then after each interval until ctx is
// canceled. Runs never overlap: the next tick waits for the current run.
func Every(ctx context.Context, interval time.Duration, job func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		job(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
