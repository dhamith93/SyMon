package main

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
)

func handleDataPurge(config *config.Collector, mysql *database.MySql) {
	ticker := time.NewTicker(6 * time.Hour)
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			select {
			case <-ticker.C:
				purgeDate := time.Now().AddDate(0, 0, -int(config.DataRetentionDays))
				unixTime := strconv.FormatInt(purgeDate.Unix(), 10)
				affectedRows, err := mysql.PurgeMonitorDataOlderThan(unixTime)
				if err != nil {
					logger.Log("error", "data-purge: "+err.Error())
				}
				logger.Log("info", "data-purge: purged "+strconv.FormatInt(affectedRows, 10)+" rows")
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	wg.Wait()
	fmt.Println("Exiting")
}
