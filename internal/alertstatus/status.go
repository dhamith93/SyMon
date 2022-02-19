package alertstatus

import "github.com/dhamith93/SyMon/internal/alerts"

type StatusType int

const (
	Normal   StatusType = 0
	Warning  StatusType = 1
	Critical StatusType = 2
)

type AlertStatus struct {
	Alert          alerts.AlertConfig
	Server         string
	UnixTime       string
	ServerTimeZone string
	Type           StatusType
	StartEvent     int64
	Value          float32
}
