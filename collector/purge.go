package main

import (
	"context"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/store"
)

// purgeResolvedAlerts deletes old resolved alerts once a day. Metric data
// is dropped by timescale retention policies instead.
func purgeResolvedAlerts(st *store.Store) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		deleted, err := st.PurgeResolvedAlerts(ctx)
		cancel()
		if err != nil {
			logger.Log("error", "alert purge: "+err.Error())
			continue
		}
		logger.Log("info", "alert purge: deleted "+strconv.FormatInt(deleted, 10)+" alerts")
	}
}
