package snmp

import (
	"maps"
	"sort"
	"strconv"
	"strings"
)

// The OIDs the profiles read, named after their MIB objects. A profile is
// a few GETs or table walks turned into Values. A device that lacks an
// object just does not get that value, only a failed request is an error.

// SNMPv2-MIB system group, read from every device
const (
	oidSysDescr    = "1.3.6.1.2.1.1.1.0"
	oidSysObjectID = "1.3.6.1.2.1.1.2.0"
	oidSysUpTime   = "1.3.6.1.2.1.1.3.0"
	oidSysName     = "1.3.6.1.2.1.1.5.0"
)

// IF-MIB: ifTable has every interface, ifXTable adds names, aliases and
// the 64 bit counters that busy links need
const (
	oidIfTable  = "1.3.6.1.2.1.2.2.1"
	oidIfXTable = "1.3.6.1.2.1.31.1.1.1"

	ifDescr       = "2"
	ifType        = "3"
	ifSpeed       = "5"
	ifAdminStatus = "7"
	ifOperStatus  = "8"
	ifInOctets    = "10"
	ifInErrors    = "14"
	ifOutOctets   = "16"
	ifOutErrors   = "20"

	ifName        = "1"
	ifHCInOctets  = "6"
	ifHCOutOctets = "10"
	ifHighSpeed   = "15"
	ifAlias       = "18"

	ifTypeEthernet = 6
	ifStatusUp     = 1
)

// HOST-RESOURCES-MIB: processor load per CPU and the storage table, whose
// rows are memory, swap and file systems told apart by type
const (
	oidHrProcessorLoad = "1.3.6.1.2.1.25.3.3.1.2"
	oidHrStorageTable  = "1.3.6.1.2.1.25.2.3.1"

	// size and used are in allocation units, which cancel out in a percent
	hrStorageType  = "2"
	hrStorageDescr = "3"
	hrStorageSize  = "5"
	hrStorageUsed  = "6"

	hrStorageRam       = "1.3.6.1.2.1.25.2.1.2"
	hrStorageFixedDisk = "1.3.6.1.2.1.25.2.1.4"
)

// UPS-MIB (RFC 1628). Input and output are tables with a row per line,
// the first line is enough for a home UPS.
const (
	oidUpsMinutesRemaining   = "1.3.6.1.2.1.33.1.2.3.0"
	oidUpsChargeRemaining    = "1.3.6.1.2.1.33.1.2.4.0"
	oidUpsBatteryTemperature = "1.3.6.1.2.1.33.1.2.7.0"
	oidUpsInputVoltage       = "1.3.6.1.2.1.33.1.3.3.1.3.1"
	oidUpsOutputSource       = "1.3.6.1.2.1.33.1.4.1.0"
	oidUpsOutputLoad         = "1.3.6.1.2.1.33.1.4.4.1.5.1"

	upsOutputSourceBattery = 5
)

// APC PowerNet-MIB, which APC network cards answer instead of UPS-MIB. A
// device whose sysObjectID is under APC's enterprise gets these.
const (
	apcEnterprise = "1.3.6.1.4.1.318."

	oidApcBatteryCapacity    = "1.3.6.1.4.1.318.1.1.1.2.2.1.0"
	oidApcBatteryTemperature = "1.3.6.1.4.1.318.1.1.1.2.2.2.0"
	oidApcRunTimeRemaining   = "1.3.6.1.4.1.318.1.1.1.2.2.3.0"
	oidApcInputLineVoltage   = "1.3.6.1.4.1.318.1.1.1.3.2.1.0"
	oidApcOutputStatus       = "1.3.6.1.4.1.318.1.1.1.4.1.1.0"
	oidApcOutputLoad         = "1.3.6.1.4.1.318.1.1.1.4.2.3.0"

	apcOutputOnBattery = 3
)

// values a session hands back: int64 for INTEGER, uint64 for the counters,
// gauges and ticks, float64 for opaque floats, string for octet strings
// and OIDs, nil when the device has no such object

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func counter(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint64:
		return n, true
	case int64:
		if n >= 0 {
			return uint64(n), true
		}
	}
	return 0, false
}

func integer(v any) int64 {
	n, _ := number(v)
	return int64(n)
}

func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// table walks a MIB table and returns its cells by row index and column,
// with the indexes in numeric order
func table(s session, root string) (map[string]map[string]any, []string, error) {
	rows := map[string]map[string]any{}
	err := s.Walk(root, func(oid string, value any) error {
		column, index, ok := strings.Cut(strings.TrimPrefix(oid, root+"."), ".")
		if !ok {
			return nil
		}
		if rows[index] == nil {
			rows[index] = map[string]any{}
		}
		rows[index][column] = value
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return rows, sortedIndexes(rows), nil
}

// column walks one column of a table and returns its cells by row index,
// with the indexes in numeric order
func column(s session, table string, col string) (map[string]any, []string, error) {
	root := table + "." + col
	cells := map[string]any{}
	err := s.Walk(root, func(oid string, value any) error {
		if index, ok := strings.CutPrefix(oid, root+"."); ok {
			cells[index] = value
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return cells, sortedIndexes(cells), nil
}

func sortedIndexes[V any](rows map[string]V) []string {
	indexes := make([]string, 0, len(rows))
	for index := range rows {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool {
		a, errA := strconv.Atoi(indexes[i])
		b, errB := strconv.Atoi(indexes[j])
		if errA == nil && errB == nil {
			return a < b
		}
		return indexes[i] < indexes[j]
	})
	return indexes
}

// readSystem fills in what the device says about itself and returns its
// sysObjectID, which tells an APC UPS apart
func readSystem(s session, report *DeviceReport) (string, error) {
	vars, err := s.Get([]string{oidSysDescr, oidSysObjectID, oidSysUpTime, oidSysName})
	if err != nil {
		return "", err
	}
	report.System.Descr = text(vars[oidSysDescr])
	report.System.Name = text(vars[oidSysName])
	if ticks, ok := counter(vars[oidSysUpTime]); ok {
		report.System.UptimeSeconds = int64(ticks / 100)
	}
	return text(vars[oidSysObjectID]), nil
}

// port is one row of the interface tables
type port struct {
	index   string
	name    string
	alias   string
	typ     int64
	adminUp bool
	up      bool
	speed   uint64
	// the counters, 64 bit when the device has them
	in, out             uint64
	inErrors, outErrors uint64
}

// readInterfaces reads the ports of a switch or router. wanted keeps only
// those names, in that order; empty keeps the physical ports. It walks the
// names, and the types when it has to find the physical ports, then gets
// the rest for the kept ports only. Some devices take most of a second for
// every request about their interface table, so walking the whole table
// took a small router half a minute.
func readInterfaces(s session, wanted []string) ([]port, error) {
	descrs, indexes, err := column(s, oidIfTable, ifDescr)
	if err != nil {
		return nil, err
	}
	// devices without ifXTable have no names, aliases or 64 bit counters
	names, _, err := column(s, oidIfXTable, ifName)
	if err != nil {
		return nil, err
	}
	var types map[string]any
	if len(wanted) == 0 {
		if types, _, err = column(s, oidIfTable, ifType); err != nil {
			return nil, err
		}
	}

	var ports []port
	for _, index := range indexes {
		p := port{index: index, name: text(names[index]), typ: integer(types[index])}
		if p.name == "" {
			p.name = text(descrs[index])
		}
		if p.name == "" {
			p.name = "if" + index
		}
		ports = append(ports, p)
	}
	ports = selectPorts(ports, wanted)

	columns := []string{oidIfTable + "." + ifSpeed, oidIfTable + "." + ifAdminStatus, oidIfTable + "." + ifOperStatus,
		oidIfTable + "." + ifInOctets, oidIfTable + "." + ifInErrors, oidIfTable + "." + ifOutOctets, oidIfTable + "." + ifOutErrors}
	if len(names) > 0 {
		columns = append(columns, oidIfXTable+"."+ifHCInOctets, oidIfXTable+"."+ifHCOutOctets, oidIfXTable+"."+ifHighSpeed, oidIfXTable+"."+ifAlias)
	}
	var oids []string
	for _, p := range ports {
		for _, col := range columns {
			oids = append(oids, col+"."+p.index)
		}
	}
	vars, err := getAll(s, oids)
	if err != nil {
		return nil, err
	}

	for i := range ports {
		p := &ports[i]
		row := func(col string) any { return vars[oidIfTable+"."+col+"."+p.index] }
		xrow := func(col string) any { return vars[oidIfXTable+"."+col+"."+p.index] }
		p.alias = text(xrow(ifAlias))
		p.adminUp = integer(row(ifAdminStatus)) == ifStatusUp
		p.up = integer(row(ifOperStatus)) == ifStatusUp
		if in, ok := counter(xrow(ifHCInOctets)); ok {
			p.in = in
			p.out, _ = counter(xrow(ifHCOutOctets))
		} else {
			p.in, _ = counter(row(ifInOctets))
			p.out, _ = counter(row(ifOutOctets))
		}
		p.inErrors, _ = counter(row(ifInErrors))
		p.outErrors, _ = counter(row(ifOutErrors))
		// ifHighSpeed is in Mbps and does not overflow at 4 Gbps like ifSpeed
		if mbps, ok := counter(xrow(ifHighSpeed)); ok && mbps > 0 {
			p.speed = mbps * 1_000_000
		} else {
			p.speed, _ = counter(row(ifSpeed))
		}
	}
	return ports, nil
}

func selectPorts(ports []port, wanted []string) []port {
	if len(wanted) > 0 {
		var picked []port
		for _, name := range wanted {
			for _, p := range ports {
				if p.name == name {
					picked = append(picked, p)
					break
				}
			}
		}
		return picked
	}
	var physical []port
	for _, p := range ports {
		if p.typ == ifTypeEthernet {
			physical = append(physical, p)
		}
	}
	// a device that calls none of its ports ethernet still shows something
	if len(physical) == 0 {
		return ports
	}
	return physical
}

// readHost reads CPU, memory and file systems from HOST-RESOURCES-MIB
func readHost(s session) ([]Value, error) {
	var loads []float64
	err := s.Walk(oidHrProcessorLoad, func(oid string, value any) error {
		if load, ok := number(value); ok {
			loads = append(loads, load)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var values []Value
	if len(loads) > 0 {
		total := 0.0
		for _, load := range loads {
			total += load
		}
		values = append(values, Value{Metric: CPUPct, Unit: unitPercent, Value: total / float64(len(loads))})
	}

	rows, indexes, err := table(s, oidHrStorageTable)
	if err != nil {
		return nil, err
	}
	memorySeen := false
	for _, index := range indexes {
		row := rows[index]
		size, _ := number(row[hrStorageSize])
		used, _ := number(row[hrStorageUsed])
		if size <= 0 {
			continue
		}
		usedPct := used / size * 100
		switch text(row[hrStorageType]) {
		case hrStorageRam:
			if !memorySeen {
				values = append(values, Value{Metric: MemUsedPct, Unit: unitPercent, Value: usedPct})
				memorySeen = true
			}
		case hrStorageFixedDisk:
			values = append(values, Value{Metric: StorageUsedPct, Label: text(row[hrStorageDescr]), Unit: unitPercent, Value: usedPct})
		}
	}
	return values, nil
}

// readUPS reads the battery and load from UPS-MIB, or from PowerNet-MIB on
// an APC card
func readUPS(s session, sysObjectID string) ([]Value, error) {
	if strings.HasPrefix(sysObjectID, apcEnterprise) {
		return readAPC(s)
	}
	vars, err := s.Get([]string{oidUpsOutputSource, oidUpsChargeRemaining, oidUpsOutputLoad, oidUpsMinutesRemaining, oidUpsInputVoltage, oidUpsBatteryTemperature})
	if err != nil {
		return nil, err
	}
	var values []Value
	if source, ok := number(vars[oidUpsOutputSource]); ok {
		values = append(values, Value{Metric: UPSOnBattery, Value: boolValue(source == upsOutputSourceBattery)})
	}
	values = appendNumber(values, UPSChargePct, unitPercent, vars[oidUpsChargeRemaining], 1)
	values = appendNumber(values, UPSLoadPct, unitPercent, vars[oidUpsOutputLoad], 1)
	values = appendNumber(values, UPSMinutesLeft, unitMinutes, vars[oidUpsMinutesRemaining], 1)
	values = appendNumber(values, UPSInputVolts, unitVolts, vars[oidUpsInputVoltage], 1)
	values = appendNumber(values, UPSBatteryCelsius, unitCelsius, vars[oidUpsBatteryTemperature], 1)
	return values, nil
}

func readAPC(s session) ([]Value, error) {
	vars, err := s.Get([]string{oidApcOutputStatus, oidApcBatteryCapacity, oidApcOutputLoad, oidApcRunTimeRemaining, oidApcInputLineVoltage, oidApcBatteryTemperature})
	if err != nil {
		return nil, err
	}
	var values []Value
	if status, ok := number(vars[oidApcOutputStatus]); ok {
		values = append(values, Value{Metric: UPSOnBattery, Value: boolValue(status == apcOutputOnBattery)})
	}
	values = appendNumber(values, UPSChargePct, unitPercent, vars[oidApcBatteryCapacity], 1)
	values = appendNumber(values, UPSLoadPct, unitPercent, vars[oidApcOutputLoad], 1)
	// the run time is in ticks, hundredths of a second
	values = appendNumber(values, UPSMinutesLeft, unitMinutes, vars[oidApcRunTimeRemaining], 1.0/6000)
	values = appendNumber(values, UPSInputVolts, unitVolts, vars[oidApcInputLineVoltage], 1)
	values = appendNumber(values, UPSBatteryCelsius, unitCelsius, vars[oidApcBatteryTemperature], 1)
	return values, nil
}

// appendNumber adds a value when the device answered it
func appendNumber(values []Value, metric string, unit string, v any, scale float64) []Value {
	if n, ok := number(v); ok {
		return append(values, Value{Metric: metric, Unit: unit, Value: n * scale})
	}
	return values
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// maxOIDsPerGet keeps a GET small enough for any device
const maxOIDsPerGet = 20

// getAll gets OIDs in batches a device can answer
func getAll(s session, oids []string) (map[string]any, error) {
	all := map[string]any{}
	for start := 0; start < len(oids); start += maxOIDsPerGet {
		vars, err := s.Get(oids[start:min(start+maxOIDsPerGet, len(oids))])
		if err != nil {
			return nil, err
		}
		maps.Copy(all, vars)
	}
	return all, nil
}

// readMetrics gets the user's own OIDs and returns what the device
// answered by metric name
func readMetrics(s session, metrics []Metric) (map[string]any, error) {
	oids := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		oids = append(oids, metric.OID)
	}
	vars, err := getAll(s, oids)
	if err != nil {
		return nil, err
	}
	answers := map[string]any{}
	for _, metric := range metrics {
		if value, ok := vars[metric.OID]; ok && value != nil {
			answers[metric.Name] = value
		}
	}
	return answers, nil
}
