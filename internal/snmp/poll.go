package snmp

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// session is what the profiles need from SNMP. The real one is gosnmp,
// tests use a map.
type session interface {
	// Get returns the values of the OIDs asked for, keyed by OID. An OID the
	// device does not have is left out or nil.
	Get(oids []string) (map[string]any, error)
	// Walk calls fn for every object under root
	Walk(root string, fn func(oid string, value any) error) error
	Close() error
}

// Poller polls devices and keeps the counters of the poll before, so
// traffic and errors come out as rates
type Poller struct {
	open func(ctx context.Context, device Device) (session, error)
	prev map[string]sample
}

// sample is the counters read from one device at one time
type sample struct {
	at     time.Time
	counts map[string]uint64
}

func NewPoller() *Poller {
	return &Poller{open: dial, prev: map[string]sample{}}
}

// Poll reads one device. It never fails: a device that cannot be read gets
// a report with Error set and no values.
func (p *Poller) Poll(ctx context.Context, device Device, now time.Time) DeviceReport {
	report := DeviceReport{
		Device:          device.Name,
		Time:            now.Unix(),
		IntervalSeconds: device.IntervalSeconds,
		Kind:            kindOf(device),
		Address:         device.Address,
	}
	s, err := p.open(ctx, device)
	if err != nil {
		report.Error = fmt.Sprintf("cannot reach %s: %v", device.Address, err)
		return report
	}
	defer s.Close()

	counts := map[string]uint64{}
	if err := p.read(s, device, &report, counts); err != nil {
		report.Error = fmt.Sprintf("cannot read %s: %v", device.Address, err)
		return DeviceReport{Device: report.Device, Time: report.Time, IntervalSeconds: report.IntervalSeconds, Kind: report.Kind, Address: report.Address, Error: report.Error}
	}
	p.prev[device.Name] = sample{at: now, counts: counts}
	return report
}

// Forget drops what was kept about a device that left the file
func (p *Poller) Forget(name string) {
	delete(p.prev, name)
}

// read runs the profiles and turns counters into rates against the poll
// before. counts collects this poll's counters for the next one.
func (p *Poller) read(s session, device Device, report *DeviceReport, counts map[string]uint64) error {
	sysObjectID, err := readSystem(s, report)
	if err != nil {
		return err
	}
	prev := p.prev[device.Name]
	elapsed := time.Unix(report.Time, 0).Sub(prev.at).Seconds()
	rate := func(key string, current uint64) (float64, bool) {
		counts[key] = current
		before, ok := prev.counts[key]
		if !ok || elapsed <= 0 || current < before {
			return 0, false
		}
		return float64(current-before) / elapsed, true
	}

	if slices.Contains(device.Profiles, ProfileInterfaces) {
		ports, err := readInterfaces(s, device.Interfaces)
		if err != nil {
			return err
		}
		for _, port := range ports {
			report.Interfaces = append(report.Interfaces, Interface{Name: port.name, Alias: port.alias, Up: port.up, AdminUp: port.adminUp, SpeedBps: int64(port.speed)})
			report.Values = append(report.Values, Value{Metric: IfUp, Label: port.name, Value: boolValue(port.up)})
			key := "if:" + port.index
			if in, ok := rate(key+":in", port.in); ok {
				report.Values = append(report.Values, Value{Metric: IfRxBps, Label: port.name, Unit: unitBps, Value: in * 8})
			}
			if out, ok := rate(key+":out", port.out); ok {
				report.Values = append(report.Values, Value{Metric: IfTxBps, Label: port.name, Unit: unitBps, Value: out * 8})
			}
			if errors, ok := rate(key+":inerrors", port.inErrors); ok {
				report.Values = append(report.Values, Value{Metric: IfRxErrors, Label: port.name, Unit: unitPerSec, Value: errors})
			}
			if errors, ok := rate(key+":outerrors", port.outErrors); ok {
				report.Values = append(report.Values, Value{Metric: IfTxErrors, Label: port.name, Unit: unitPerSec, Value: errors})
			}
		}
	}
	if slices.Contains(device.Profiles, ProfileHost) {
		values, err := readHost(s)
		if err != nil {
			return err
		}
		report.Values = append(report.Values, values...)
	}
	if slices.Contains(device.Profiles, ProfileUPS) {
		values, err := readUPS(s, sysObjectID)
		if err != nil {
			return err
		}
		report.Values = append(report.Values, values...)
	}
	if len(device.Metrics) > 0 {
		answers, err := readMetrics(s, device.Metrics)
		if err != nil {
			return err
		}
		for _, metric := range device.Metrics {
			answer, ok := answers[metric.Name]
			if !ok {
				continue
			}
			scale := metric.Scale
			if scale == 0 {
				scale = 1
			}
			if metric.Counter {
				current, ok := counter(answer)
				if !ok {
					continue
				}
				if perSecond, ok := rate("metric:"+metric.Name, current); ok {
					report.Values = append(report.Values, Value{Metric: metric.Name, Unit: metric.Unit, Value: perSecond * scale})
				}
				continue
			}
			if n, ok := number(answer); ok {
				report.Values = append(report.Values, Value{Metric: metric.Name, Unit: metric.Unit, Value: n * scale})
			}
		}
	}
	return nil
}
