package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/send"
)

// "log"
// "os"

// "github.com/dhamith93/SyMon/agent/monitor"
// "github.com/dhamith93/SyMon/internal/config"

//https://github.com/shirou/gopsutil

func main() {
	if config.GetConfig("config.json").LogFileEnabled {
		file, err := os.OpenFile(config.GetConfig("config.json").LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	config := config.GetConfig("config.json")

	ticker := time.NewTicker(60 * time.Second)
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			select {
			case <-ticker.C:
				monitorData := monitor.MonitorAsJSON(config)
				send.SendPost(config.MonitorEndpoint, monitorData)
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	wg.Wait()
	fmt.Println("Exiting")
}
