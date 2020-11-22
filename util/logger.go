package util

import (
	"log"
)

// Log logs to given log file
func Log(prefix string, msg string) {
	if !GetConfig().LogFileEnabled {
		return
	}
	log.Println(prefix + " " + msg)
}
