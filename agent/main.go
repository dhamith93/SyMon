package main

import (
	"log"
	"os"

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
	monitorData := monitor.MonitorAsJSON(config)
	send.SendPost(config.MonitorEndpoint, monitorData)
}
