package main

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/snmp"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SNMP devices. This agent polls the switches, UPSs and other boxes in its
// devices file and reports each to the collector. The file is read every
// round, so devices can be added without a restart, and no file means
// nothing to do.

const (
	// how long one device's poll may take, under the shortest interval
	snmpPollTimeout = 45 * time.Second
	// how long to leave a collector from before devices alone
	snmpOldCollector = 10 * time.Minute
	// the shortest interval the agent polls at
	snmpMinInterval = 10 * time.Second
)

type deviceClient interface {
	ReportDevice(ctx context.Context, in *api.DeviceReport, opts ...grpc.CallOption) (*api.Message, error)
}

type devicePoller struct {
	path     string
	host     string
	interval time.Duration
	poller   interface {
		Poll(ctx context.Context, device snmp.Device, now time.Time) snmp.DeviceReport
		Forget(name string)
	}
	client deviceClient
	load   func(path string) ([]snmp.Device, error)
	now    func() time.Time
	// what was last logged about the file, each device's poll and each
	// device's report, so a problem is logged once and its end once
	fileError    string
	pollErrors   map[string]string
	reportErrors map[string]string
	// due is when each device is next polled
	due map[string]time.Time
}

func newDevicePoller(config *config.Agent, client deviceClient) *devicePoller {
	interval := max(time.Duration(config.SNMPIntervalSeconds)*time.Second, snmpMinInterval)
	return &devicePoller{
		path:         config.SNMPDevicesPath,
		host:         config.ServerId,
		interval:     interval,
		poller:       snmp.NewPoller(),
		client:       client,
		load:         snmp.Load,
		now:          time.Now,
		pollErrors:   map[string]string{},
		reportErrors: map[string]string{},
		due:          map[string]time.Time{},
	}
}

func (p *devicePoller) loop() {
	for {
		time.Sleep(p.step())
	}
}

// step polls the devices that are due and reports them. It returns how long
// to wait before the next step.
func (p *devicePoller) step() time.Duration {
	now := p.now()
	devices, err := p.load(p.path)
	message := ""
	if err != nil {
		message = err.Error()
	}
	if message != p.fileError {
		switch {
		case message != "":
			logger.Log("error", "not polling devices: "+message)
		case p.fileError != "":
			logger.Log("info", "polling the devices in "+p.path+" again")
		}
		p.fileError = message
	}
	if err != nil {
		return p.interval
	}

	// forget devices that left the file
	current := map[string]bool{}
	for _, device := range devices {
		current[device.Name] = true
	}
	for name := range p.due {
		if !current[name] {
			p.poller.Forget(name)
			delete(p.due, name)
			delete(p.pollErrors, name)
			delete(p.reportErrors, name)
		}
	}
	if len(devices) != len(p.due) {
		logger.Log("info", "polling "+strconv.Itoa(len(devices))+" devices from "+p.path)
	}

	var due []snmp.Device
	for _, device := range devices {
		if device.IntervalSeconds == 0 {
			device.IntervalSeconds = int(p.interval.Seconds())
		}
		if next, ok := p.due[device.Name]; ok && now.Before(next) {
			continue
		}
		p.due[device.Name] = now.Add(time.Duration(device.IntervalSeconds) * time.Second)
		due = append(due, device)
	}
	for _, report := range p.poll(due, now) {
		if report.Error != p.pollErrors[report.Device] {
			if report.Error != "" {
				logger.Log("error", "device "+report.Device+": "+report.Error)
			} else {
				logger.Log("info", "device "+report.Device+" answers again")
			}
			p.pollErrors[report.Device] = report.Error
		}
		if tooOld := p.report(report); tooOld {
			return snmpOldCollector
		}
	}

	// wake for the device that is due first
	wait := p.interval
	for _, next := range p.due {
		wait = min(wait, next.Sub(now))
	}
	return max(wait, time.Second)
}

// poll reads the devices at the same time, so a dead one does not hold up
// the others
func (p *devicePoller) poll(devices []snmp.Device, now time.Time) []snmp.DeviceReport {
	reports := make([]snmp.DeviceReport, len(devices))
	var wg sync.WaitGroup
	for i, device := range devices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), snmpPollTimeout)
			defer cancel()
			reports[i] = p.poller.Poll(ctx, device, now)
		}()
	}
	wg.Wait()
	return reports
}

// report sends one report. It returns true when the collector does not
// know devices at all.
func (p *devicePoller) report(report snmp.DeviceReport) bool {
	data, err := json.Marshal(report)
	if err != nil {
		logger.Log("error", "cannot encode the report of "+report.Device+": "+err.Error())
		return false
	}
	ctx, cancel := transport.Context()
	_, err = p.client.ReportDevice(ctx, &api.DeviceReport{Host: p.host, ReportJson: string(data)})
	cancel()
	if status.Code(err) == codes.Unimplemented || status.Code(err) == codes.PermissionDenied {
		// a collector from before devices does not know the call, or does
		// not let agents make it
		logger.Log("error", "the collector is too old for devices, update it to see "+report.Device)
		return true
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	if message != p.reportErrors[report.Device] {
		if message != "" {
			logger.Log("error", "cannot report the device "+report.Device+": "+message)
		} else {
			logger.Log("info", "reporting the device "+report.Device+" again")
		}
		p.reportErrors[report.Device] = message
	}
	return false
}
