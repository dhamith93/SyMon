package util

import (
	"log"
)

func Log(prefix string, msg string) {
	if !GetConfig().LogFileEnabled {
		return
	}
	log.Println(prefix + " " + msg)
}
