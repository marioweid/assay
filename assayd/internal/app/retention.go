package app

import (
	"context"
	"time"
)

func (a *App) maintainPartitions(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		maintenanceCtx, cancel := context.WithTimeout(ctx, time.Minute)
		err := a.database.MaintainPartitions(maintenanceCtx, time.Now(), a.config.TraceRetentionDays)
		cancel()
		if err != nil && ctx.Err() == nil {
			a.logger.Error("partition maintenance failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
