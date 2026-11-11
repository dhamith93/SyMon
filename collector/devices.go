package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
)

// deviceTarget is a device, port or value a device rule checks. A target
// is the device's name, or device/port or device/label.
type deviceTarget struct {
	target string
	value  float64
	at     time.Time
	// reason is the device's error, a port's alias or a value's unit, for
	// the message
	reason string
}

// evaluateDevice checks a device rule against the newest report of its
// device, or of every device with "*". A device whose agent stopped
// reporting is no data, so nothing opens or resolves for it until the agent
// is back; the host's own ping rule covers the agent. An open alert whose
// device, port or value is gone resolves.
func (e *evaluator) evaluateDevice(ctx context.Context, rule *alerts.AlertConfig) error {
	devices, err := e.ruleDevices(ctx, rule)
	if err != nil {
		return err
	}
	now := e.now()
	present := map[string]bool{}
	for _, device := range devices {
		targets := deviceTargets(rule, device)
		for _, t := range targets {
			present[t.target] = true
		}
		if device.Stale(now) {
			continue
		}
		for _, t := range targets {
			e.reasons[rule.Name+"|"+t.target] = t.reason
			if err := e.check(ctx, rule, "", t.target, t.value, t.at); err != nil {
				return err
			}
		}
	}
	return e.resolveGone(ctx, rule, present, now)
}

// ruleDevices are the devices a rule checks. An unknown device is no data.
func (e *evaluator) ruleDevices(ctx context.Context, rule *alerts.AlertConfig) ([]store.Device, error) {
	if rule.Device == alerts.AllHosts {
		return e.store.Devices(ctx)
	}
	device, err := e.store.Device(ctx, rule.Device)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []store.Device{device}, nil
}

// deviceTargets are what a rule checks in one device. Whether the device
// answers is checked against its last poll, ports and values against the
// last poll that could read it, so an unreachable device changes nothing
// about them.
func deviceTargets(rule *alerts.AlertConfig, device store.Device) []deviceTarget {
	state := device.State
	read := time.Unix(state.Time, 0)
	switch rule.MetricName {
	case monitor.DEVICE_DOWN:
		value := 0.0
		if device.Error != "" {
			value = 1
		}
		return []deviceTarget{{target: device.Name, value: value, at: device.ReportedAt, reason: device.Error}}
	case monitor.DEVICE_PORT:
		for _, port := range state.Interfaces {
			if port.Name != rule.Interface {
				continue
			}
			value := 0.0
			if port.AdminUp && !port.Up {
				value = 1
			}
			return []deviceTarget{{target: device.Name + "/" + port.Name, value: value, at: read, reason: port.Alias}}
		}
		return nil
	}
	var targets []deviceTarget
	for _, v := range state.Values {
		if v.Metric != rule.DeviceMetric || (rule.Label != "" && v.Label != rule.Label) {
			continue
		}
		target := device.Name
		if v.Label != "" {
			target += "/" + v.Label
		}
		targets = append(targets, deviceTarget{target: target, value: v.Value, at: read, reason: v.Unit})
	}
	return targets
}

// splitTarget takes a device rule's target apart. Device names cannot
// contain /, so the first one ends the device.
func splitTarget(target string) (device string, part string) {
	device, part, _ = strings.Cut(target, "/")
	return device, part
}

// deviceSubject is the device, port or value a rule's target is, in words
func deviceSubject(rule *alerts.AlertConfig, target string, alias string) string {
	device, part := splitTarget(target)
	switch rule.MetricName {
	case monitor.DEVICE_DOWN:
		return device
	case monitor.DEVICE_PORT:
		if alias != "" {
			return "port " + part + " (" + alias + ") on " + device
		}
		return "port " + part + " on " + device
	}
	if part != "" {
		return rule.DeviceMetric + " of " + part + " on " + device
	}
	return rule.DeviceMetric + " on " + device
}

// withUnit writes a value with its unit, like 18% or 230 V
func withUnit(value float64, unit string) string {
	text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
	switch unit {
	case "":
		return text
	case "%":
		return text + unit
	}
	return text + " " + unit
}

// deviceText fills a device rule's template. reason is the device's error,
// the port's alias or the value's unit.
func deviceText(rule *alerts.AlertConfig, target string, reason string, status alertstatus.StatusType, value float64, at time.Time) (string, string) {
	unit := ""
	if rule.MetricName == monitor.DEVICE_METRIC {
		unit = reason
	}
	subject := deviceSubject(rule, target, reason)
	if status == alertstatus.Normal {
		var fine string
		switch rule.MetricName {
		case monitor.DEVICE_DOWN:
			fine = subject + " answers again"
		case monitor.DEVICE_PORT:
			fine = subject + " is up again"
		default:
			fine = subject + " is back at " + withUnit(value, unit)
		}
		return "[Resolved] " + rule.Name + ": " + fine, fine + " at " + at.UTC().Format(time.RFC1123) + "."
	}

	level, expected := "[Critical] ", rule.CriticalThreshold
	if status == alertstatus.Warning {
		level, expected = "[Warning] ", rule.WarnThreshold
	}
	var problem string
	switch rule.MetricName {
	case monitor.DEVICE_DOWN:
		problem = subject + " is unreachable"
	case monitor.DEVICE_PORT:
		problem = subject + " is down"
	default:
		problem = subject + " is " + withUnit(value, unit) + " " + rule.Op + " " + withUnit(float64(expected), unit)
	}
	device, _ := splitTarget(target)
	heading := level + rule.Name + ": " + problem
	replacer := strings.NewReplacer(
		"{subject}", heading,
		"{device}", device,
		"{target}", target,
		"{metricName}", rule.MetricName,
		"{op}", rule.Op,
		"{expected}", strconv.Itoa(expected),
		"{value}", fmt.Sprintf("%.2f", value),
		"{reason}", reason,
		"{timestamp}", at.UTC().String(),
		"{desc}", rule.Description,
		"{triggerInterval}", strconv.Itoa(rule.TriggerIntveral),
	)
	body := replacer.Replace(rule.Template)
	if strings.TrimSpace(body) == "" {
		body = problem + "."
		if rule.MetricName == monitor.DEVICE_DOWN && reason != "" {
			body = problem + ": " + reason + "."
		}
	}
	return heading, body
}

func deviceGoneText(rule *alerts.AlertConfig, target string, at time.Time) (string, string) {
	subject := deviceSubject(rule, target, "")
	return "[Resolved] " + rule.Name + ": " + subject + " is gone",
		subject + " is not reported any more, as of " + at.UTC().Format(time.RFC1123) + "."
}
