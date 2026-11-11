// Package snmp reads network devices over SNMP: switches and routers, UPSs,
// NAS boxes and anything else that answers a few well-known OIDs. The agent
// on one LAN host polls the devices in its devices file and reports what it
// read to the collector.
package snmp

// The metrics the profiles produce. Custom OIDs are named by the user.
const (
	IfRxBps    = "if_rx_bps"
	IfTxBps    = "if_tx_bps"
	IfRxErrors = "if_rx_errors_ps"
	IfTxErrors = "if_tx_errors_ps"
	IfUp       = "if_up"

	CPUPct         = "cpu_pct"
	MemUsedPct     = "mem_used_pct"
	StorageUsedPct = "storage_used_pct"

	UPSOnBattery      = "ups_on_battery"
	UPSChargePct      = "ups_charge_pct"
	UPSLoadPct        = "ups_load_pct"
	UPSMinutesLeft    = "ups_minutes_left"
	UPSInputVolts     = "ups_input_volts"
	UPSBatteryCelsius = "ups_battery_celsius"
)

// builtinMetrics are the names the profiles use, which a custom OID cannot
// take
var builtinMetrics = map[string]bool{
	IfRxBps: true, IfTxBps: true, IfRxErrors: true, IfTxErrors: true, IfUp: true,
	CPUPct: true, MemUsedPct: true, StorageUsedPct: true,
	UPSOnBattery: true, UPSChargePct: true, UPSLoadPct: true, UPSMinutesLeft: true, UPSInputVolts: true, UPSBatteryCelsius: true,
}

// units the profiles report
const (
	unitBps     = "bps"
	unitPerSec  = "/s"
	unitPercent = "%"
	unitMinutes = "min"
	unitVolts   = "V"
	unitCelsius = "°C"
)

// What a device is, worked out from its profiles unless the file says
const (
	KindUPS    = "ups"
	KindSwitch = "switch"
	KindServer = "server"
	KindOther  = "other"
)

// DeviceReport is what the agent read from one device in one poll. It never
// holds the community. The collector keeps the newest one per device and
// every Value as a series.
type DeviceReport struct {
	Device string
	// Host is the agent that polled, set by the collector from the agent's key
	Host string `json:",omitempty"`
	// Time is when the poll happened, unix seconds
	Time int64
	// IntervalSeconds is how often this device is polled, so the collector
	// knows when a report is overdue
	IntervalSeconds int
	// Error is why the device could not be read, empty when it could. A
	// report with an error has no values.
	Error      string `json:",omitempty"`
	Kind       string
	Address    string
	System     System
	Interfaces []Interface `json:",omitempty"`
	Values     []Value     `json:",omitempty"`
}

// System is what every device says about itself
type System struct {
	Descr         string `json:",omitempty"`
	Name          string `json:",omitempty"`
	UptimeSeconds int64  `json:",omitempty"`
}

// Interface is a port of a switch or router, for the ports table. Its
// traffic is in Values, labelled with the port's name.
type Interface struct {
	Name  string
	Alias string `json:",omitempty"`
	// Up is the link, AdminUp whether the port is switched on
	Up       bool
	AdminUp  bool
	SpeedBps int64 `json:",omitempty"`
}

// Value is one number read from the device. Label tells rows of a table
// apart, like ports or volumes, and is empty for a scalar.
type Value struct {
	Metric string
	Label  string `json:",omitempty"`
	Unit   string `json:",omitempty"`
	Value  float64
}
