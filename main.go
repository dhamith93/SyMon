package main

import (
	// "encoding/json"
	// "fmt"
	// "symon/config"
	// "symon/monitor"
	"symon/server"
)

func main() {
	// conf := config.Config{MonitorInterval: 5, LogFileEnabled: true, LogFilePath: "/path/to", DBPath: "/path/to"}
	server.Run()
}
