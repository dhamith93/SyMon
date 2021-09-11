package monitor

import (
	"fmt"
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
	"github.com/dhamith93/SyMon/internal/config"
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

func GetServices(unixTime string, config config.Config) []Service {
	servicesToCheck := config.Services
	var services []Service
	for _, serviceToCheck := range servicesToCheck {
		services = append(services, Service{
			Name:    serviceToCheck.Name,
			Running: IsServiceUp(serviceToCheck.ServiceName),
			Time:    unixTime,
		})
	}
	fmt.Println(services)
	return services
}
