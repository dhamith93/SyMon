package logger

import (
	"log"

	"github.com/dhamith93/SyMon/internal/config"
)

func Log(prefix string, msg string) {
	if !config.LogFileEnabled() {
		return
	}
	log.Println(prefix + " " + msg)
}
