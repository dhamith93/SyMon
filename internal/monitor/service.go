package monitor

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
)

// Service holds service activity info
type Service struct {
	Name    string
	Running bool
	Time    string
}

// IsServiceUp returns true if service is running
func IsServiceUp(serviceName string) bool {
	return strings.TrimSpace(command.Execute("systemctl is-active "+serviceName, true)) == "active"
}
