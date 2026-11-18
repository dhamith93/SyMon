package server

import (
	"net/http"

	"github.com/dhamith93/SyMon/internal/api"
)

// the process activity of a host over a range, from the agent's eBPF tracer

type activityTotals struct {
	Seconds      float64 `json:"seconds"`
	Execs        int64   `json:"execs"`
	FailedExecs  int64   `json:"failedExecs"`
	Exits        int64   `json:"exits"`
	NonZeroExits int64   `json:"nonZeroExits"`
	Signaled     int64   `json:"signaled"`
	Dropped      int64   `json:"dropped"`
	Listed       int64   `json:"listed"`
	OOM          int64   `json:"oom"`
	Crashes      int64   `json:"crashes"`
}

type exitGroup struct {
	Name            string  `json:"name"`
	Status          int32   `json:"status"`
	Signal          int32   `json:"signal"`
	CoreDumped      bool    `json:"coreDumped"`
	OOM             bool    `json:"oom"`
	Count           int64   `json:"count"`
	First           int64   `json:"first"`
	Last            int64   `json:"last"`
	Parent          string  `json:"parent"`
	LifetimeSeconds float64 `json:"lifetimeSeconds"`
}

type processExit struct {
	Time            int64   `json:"time"`
	Name            string  `json:"name"`
	Parent          string  `json:"parent"`
	Pid             int32   `json:"pid"`
	Ppid            int32   `json:"ppid"`
	Uid             int64   `json:"uid"`
	Status          int32   `json:"status"`
	Signal          int32   `json:"signal"`
	CoreDumped      bool    `json:"coreDumped"`
	OOM             bool    `json:"oom"`
	LifetimeSeconds float64 `json:"lifetimeSeconds"`
}

// getProcessActivity returns the tracer's counts over a range, the programs
// that ended badly grouped by how, and the newest of them
func (s *server) getProcessActivity(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	query := r.URL.Query()
	from, to, err := timeRange(query.Get("from"), query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response, err := s.collector.ProcessActivity(r.Context(), &api.ProcessActivityRequest{Host: host, From: from, To: to})
	if err != nil {
		writeGRPCError(w, "process activity of "+host, err)
		return
	}

	totals := activityTotals{}
	if t := response.Totals; t != nil {
		totals = activityTotals{
			Seconds: t.Seconds, Execs: t.Execs, FailedExecs: t.FailedExecs, Exits: t.Exits, NonZeroExits: t.NonZeroExits,
			Signaled: t.Signaled, Dropped: t.Dropped, Listed: t.Listed, OOM: t.Oom, Crashes: t.Crashes,
		}
	}
	groups := make([]exitGroup, 0, len(response.Groups))
	for _, g := range response.Groups {
		groups = append(groups, exitGroup{
			Name: g.Name, Status: g.Status, Signal: g.Signal, CoreDumped: g.CoreDumped, OOM: g.Oom,
			Count: g.Count, First: g.First, Last: g.Last, Parent: g.Parent, LifetimeSeconds: g.LifetimeSeconds,
		})
	}
	latest := make([]processExit, 0, len(response.Latest))
	for _, e := range response.Latest {
		latest = append(latest, processExit{
			Time: e.Time, Name: e.Name, Parent: e.Parent, Pid: e.Pid, Ppid: e.Ppid, Uid: e.Uid,
			Status: e.Status, Signal: e.Signal, CoreDumped: e.CoreDumped, OOM: e.Oom, LifetimeSeconds: e.LifetimeSeconds,
		})
	}
	writeJSON(w, map[string]any{
		"totals":    totals,
		"groups":    groups,
		"latest":    latest,
		"exitsFrom": response.ExitsFrom,
	})
}
