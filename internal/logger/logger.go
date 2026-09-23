package logger

import (
	"log"
)

// Log writes to the standard logger. main sets its output to a file when
// log files are enabled, otherwise it goes to stderr.
func Log(prefix string, msg string) {
	log.Println(prefix + " " + msg)
}
