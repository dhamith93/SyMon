package main

import (
	"context"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/store"
)

// finished task runs are kept this long unless SYMON_TASK_HISTORY_DAYS says otherwise
const defaultTaskHistoryDays = 90

func taskHistory(config *config.Collector) time.Duration {
	if config.TaskHistoryDays > 0 {
		return days(config.TaskHistoryDays)
	}
	return days(defaultTaskHistoryDays)
}

// sweepTaskRuns ends the task runs nobody will report on: the ones no
// agent picked up in time, and the ones whose host went away mid run
func sweepTaskRuns(st *store.Store) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		ended, err := st.SweepTaskRuns(ctx)
		cancel()
		if err != nil {
			logger.Log("error", "task sweep: "+err.Error())
			continue
		}
		if ended > 0 {
			logger.Log("info", "ended "+strconv.FormatInt(ended, 10)+" task runs that were not picked up or went silent")
		}
	}
}
