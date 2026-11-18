package server

import (
	"net/http"

	"github.com/dhamith93/SyMon/internal/api"
)

// a disk's latency histogram over a range, from the agent's eBPF counters

type latencyStep struct {
	Time    int64   `json:"time"`
	Buckets []int64 `json:"buckets"`
	P50     float64 `json:"p50"`
	P95     float64 `json:"p95"`
	P99     float64 `json:"p99"`
}

// getDiskLatency returns the histogram of one disk's reads or writes, and
// the disks that have one. ?device= picks the disk, the busiest without it,
// and ?op= is read (the default) or write.
func (s *server) getDiskLatency(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	query := r.URL.Query()
	from, to, err := timeRange(query.Get("from"), query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	op := query.Get("op")
	if op == "" {
		op = "read"
	}
	response, err := s.collector.DiskLatency(r.Context(), &api.DiskLatencyRequest{Host: host, Device: query.Get("device"), Op: op, From: from, To: to})
	if err != nil {
		writeGRPCError(w, "disk latency of "+host, err)
		return
	}

	steps := make([]latencyStep, 0, len(response.Steps))
	for _, step := range response.Steps {
		steps = append(steps, latencyStep{Time: step.Time, Buckets: step.Buckets, P50: step.P50, P95: step.P95, P99: step.P99})
	}
	devices := response.Devices
	if devices == nil {
		devices = []string{}
	}
	writeJSON(w, map[string]any{
		"devices":      devices,
		"device":       response.Device,
		"op":           response.Op,
		"stepSeconds":  response.StepSeconds,
		"steps":        steps,
		"bucketStarts": response.BucketStarts,
	})
}
