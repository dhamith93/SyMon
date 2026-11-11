package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dhamith93/SyMon/internal/api"
)

// SNMP devices, as the agent that polls each reports them. Viewers see all
// of it: community strings stay in the devices file on the polling host.

type deviceSummary struct {
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Address         string  `json:"address"`
	Host            string  `json:"host"`
	ReportedAt      int64   `json:"reportedAt"`
	IntervalSeconds int32   `json:"intervalSeconds"`
	Error           string  `json:"error"`
	Stale           bool    `json:"stale"`
	UptimeSeconds   int64   `json:"uptimeSeconds"`
	Ports           int32   `json:"ports"`
	PortsUp         int32   `json:"portsUp"`
	RxBps           float64 `json:"rxBps"`
	TxBps           float64 `json:"txBps"`
	// null when the device does not report them
	CPUPct         *float64 `json:"cpuPct"`
	MemUsedPct     *float64 `json:"memUsedPct"`
	UPSChargePct   *float64 `json:"upsChargePct"`
	UPSLoadPct     *float64 `json:"upsLoadPct"`
	UPSMinutesLeft *float64 `json:"upsMinutesLeft"`
	UPSOnBattery   *bool    `json:"upsOnBattery"`
}

func deviceSummaryOf(d *api.DeviceSummary) deviceSummary {
	return deviceSummary{
		Name:            d.Name,
		Kind:            d.Kind,
		Address:         d.Address,
		Host:            d.Host,
		ReportedAt:      d.ReportedAt,
		IntervalSeconds: d.IntervalSeconds,
		Error:           d.Error,
		Stale:           d.Stale,
		UptimeSeconds:   d.UptimeSeconds,
		Ports:           d.Ports,
		PortsUp:         d.PortsUp,
		RxBps:           d.RxBps,
		TxBps:           d.TxBps,
		CPUPct:          d.CpuPct,
		MemUsedPct:      d.MemUsedPct,
		UPSChargePct:    d.UpsChargePct,
		UPSLoadPct:      d.UpsLoadPct,
		UPSMinutesLeft:  d.UpsMinutesLeft,
		UPSOnBattery:    d.UpsOnBattery,
	}
}

func (s *server) getDevices(w http.ResponseWriter, r *http.Request) {
	response, err := s.collector.Devices(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "devices", err)
		return
	}
	devices := make([]deviceSummary, 0, len(response.Devices))
	for _, d := range response.Devices {
		devices = append(devices, deviceSummaryOf(d))
	}
	writeJSON(w, map[string]any{"devices": devices})
}

// getDevice returns the newest report that could read the device
func (s *server) getDevice(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	info, err := s.collector.Device(r.Context(), &api.DeviceRequest{Name: name})
	if err != nil {
		writeGRPCError(w, "device "+name, err)
		return
	}
	writeJSON(w, map[string]any{
		"summary": deviceSummaryOf(info.Summary),
		"state":   json.RawMessage(info.StateJson),
	})
}

func (s *server) getDeviceSeries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	query := r.URL.Query()
	from, to, err := timeRange(query.Get("from"), query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxPoints, _ := strconv.Atoi(query.Get("maxPoints"))
	response, err := s.collector.DeviceSeries(r.Context(), &api.DeviceSeriesRequest{
		Name:      name,
		Metric:    query.Get("metric"),
		From:      from,
		To:        to,
		MaxPoints: int32(maxPoints),
		// label may repeat, to chart a few ports of a big switch
		Labels: query["label"],
	})
	if err != nil {
		writeGRPCError(w, query.Get("metric")+" for device "+name, err)
		return
	}
	writeSeries(w, response)
}

// deleteDevice forgets a device, so another host's agent may poll it
func (s *server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.collector.RemoveDevice(r.Context(), &api.DeviceRequest{Name: name}); err != nil {
		writeGRPCError(w, "removing device "+name, err)
		return
	}
	writeJSON(w, map[string]string{"name": name})
}
