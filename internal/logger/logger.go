package logger

import (
	"log"

	"github.com/dhamith93/SyMon/internal/config"
)

// Log logs to given log file
func Log(prefix string, msg string) {
	if !config.GetConfig("config.json").LogFileEnabled {
		return
	}
	log.Println(prefix + " " + msg)
}
