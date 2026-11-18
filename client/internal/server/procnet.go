package server

import (
	"net/http"

	"github.com/dhamith93/SyMon/internal/api"
)

// what each program on a host received and sent over a range, from the
// agent's eBPF counters

type programTraffic struct {
	Name    string  `json:"name"`
	RxBytes int64   `json:"rxBytes"`
	TxBytes int64   `json:"txBytes"`
	RxPeak  float64 `json:"rxPeak"`
	TxPeak  float64 `json:"txPeak"`
}

// getProcessNetwork returns every program's traffic added up, and the
// busiest programs
func (s *server) getProcessNetwork(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	query := r.URL.Query()
	from, to, err := timeRange(query.Get("from"), query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response, err := s.collector.ProcessNetwork(r.Context(), &api.ProcessNetworkRequest{Host: host, From: from, To: to})
	if err != nil {
		writeGRPCError(w, "process network of "+host, err)
		return
	}

	programs := make([]programTraffic, 0, len(response.Programs))
	for _, p := range response.Programs {
		programs = append(programs, programTraffic{Name: p.Name, RxBytes: p.RxBytes, TxBytes: p.TxBytes, RxPeak: p.RxPeak, TxPeak: p.TxPeak})
	}
	writeJSON(w, map[string]any{
		"rxBytes":  response.RxBytes,
		"txBytes":  response.TxBytes,
		"seconds":  response.Seconds,
		"programs": programs,
	})
}
