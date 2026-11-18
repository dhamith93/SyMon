package store

import (
	"context"
	"fmt"
	"time"
)

// crashSignals end a program that went wrong by itself: SIGILL, SIGTRAP,
// SIGABRT, SIGBUS, SIGFPE, SIGSEGV and SIGSYS. A core dump counts too.
var crashSignals = []int16{4, 5, 6, 7, 8, 11, 31}

// sigkill is what the OOM killer and kill -9 send
const sigkill = 9

// how many exit groups and exits ProcessActivity returns at most
const (
	exitGroupLimit  = 100
	latestExitLimit = 100
)

// ProcessActivityTotals adds up a host's process activity over a range.
// Seconds is how much of it the tracer covered.
type ProcessActivityTotals struct {
	Seconds      float64
	Execs        int64
	FailedExecs  int64
	Exits        int64
	NonZeroExits int64
	Signaled     int64
	// abnormal exits that were counted but not kept
	Dropped int64
	// from the kept exits since ExitsFrom
	Listed  int64
	OOM     int64
	Crashes int64
}

// ExitGroup is one program that ended the same way, how often and when
type ExitGroup struct {
	Name       string
	Status     int
	Signal     int
	CoreDumped bool
	OOM        bool
	Count      int64
	First      time.Time
	Last       time.Time
	// the parent seen most, and the median lifetime
	Parent          string
	LifetimeSeconds float64
}

// ProcessExit is one process that ended badly
type ProcessExit struct {
	Time            time.Time
	Name            string
	Parent          string
	Pid             int
	Ppid            int
	Uid             int64
	Status          int
	Signal          int
	CoreDumped      bool
	OOM             bool
	LifetimeSeconds float64
}

type ProcessActivityResult struct {
	Totals ProcessActivityTotals
	// the worst first, then the most frequent
	Groups []ExitGroup
	// newest first
	Latest []ProcessExit
	// exits are kept like raw data, so they are listed from here on
	ExitsFrom time.Time
}

// ProcessActivity returns a host's process activity over a range: the
// counts, the programs that ended badly grouped by how, and the newest of
// them
func (s *Store) ProcessActivity(ctx context.Context, host string, from time.Time, to time.Time) (ProcessActivityResult, error) {
	if !to.After(from) {
		return ProcessActivityResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return ProcessActivityResult{}, err
	}

	now := time.Now()
	result := ProcessActivityResult{Groups: []ExitGroup{}, Latest: []ProcessExit{}, ExitsFrom: from}
	if keptFrom := now.Add(-s.retention.Raw); keptFrom.After(from) {
		result.ExitsFrom = keptFrom
	}

	// the same source as the charts, so the totals match them
	source, _ := pickSource(seriesDefs["proc_started"], from, to, now, defaultMaxPoints, s.retention)
	table, timeColumn := "process_activity", "time"
	if source != "raw" {
		table, timeColumn = "process_activity_"+source, "bucket"
	}
	t := &result.Totals
	err = s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT coalesce(sum(seconds), 0)::float8, coalesce(sum(execs), 0)::bigint, coalesce(sum(failed_execs), 0)::bigint,
		       coalesce(sum(exits), 0)::bigint, coalesce(sum(nonzero_exits), 0)::bigint, coalesce(sum(signaled), 0)::bigint,
		       coalesce(sum(dropped), 0)::bigint
		FROM %[1]s
		WHERE host_id = $1 AND %[2]s >= $2 AND %[2]s < $3`, table, timeColumn),
		hostID, from, to).Scan(&t.Seconds, &t.Execs, &t.FailedExecs, &t.Exits, &t.NonZeroExits, &t.Signaled, &t.Dropped)
	if err != nil {
		return result, err
	}

	exitsFrom := result.ExitsFrom
	if !to.After(exitsFrom) {
		return result, nil
	}
	err = s.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE oom),
		       count(*) FILTER (WHERE NOT oom AND (core_dumped OR signal = ANY($4)))
		FROM process_exits
		WHERE host_id = $1 AND time >= $2 AND time < $3`,
		hostID, exitsFrom, to, crashSignals).Scan(&t.Listed, &t.OOM, &t.Crashes)
	if err != nil {
		return result, err
	}
	if t.Listed == 0 {
		return result, nil
	}

	// the worst kinds first, so a single OOM kill is never cut off by a
	// script that fails every minute
	rows, err := s.pool.Query(ctx, `
		SELECT name, status, signal, core_dumped, oom, count(*), min(time), max(time),
		       mode() WITHIN GROUP (ORDER BY parent),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY lifetime_seconds)
		FROM process_exits
		WHERE host_id = $1 AND time >= $2 AND time < $3
		GROUP BY name, status, signal, core_dumped, oom
		ORDER BY oom DESC, (core_dumped OR signal = ANY($4)) DESC, signal = $5 DESC, count(*) DESC, max(time) DESC
		LIMIT $6`,
		hostID, exitsFrom, to, crashSignals, sigkill, exitGroupLimit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var g ExitGroup
		var status, signal int16
		if err := rows.Scan(&g.Name, &status, &signal, &g.CoreDumped, &g.OOM, &g.Count, &g.First, &g.Last, &g.Parent, &g.LifetimeSeconds); err != nil {
			return result, err
		}
		g.Status, g.Signal = int(status), int(signal)
		result.Groups = append(result.Groups, g)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT time, name, parent, pid, ppid, uid, status, signal, core_dumped, oom, lifetime_seconds
		FROM process_exits
		WHERE host_id = $1 AND time >= $2 AND time < $3
		ORDER BY time DESC
		LIMIT $4`,
		hostID, exitsFrom, to, latestExitLimit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var e ProcessExit
		var pid, ppid int32
		var status, signal int16
		var lifetime float32
		if err := rows.Scan(&e.Time, &e.Name, &e.Parent, &pid, &ppid, &e.Uid, &status, &signal, &e.CoreDumped, &e.OOM, &lifetime); err != nil {
			return result, err
		}
		e.Pid, e.Ppid, e.Status, e.Signal, e.LifetimeSeconds = int(pid), int(ppid), int(status), int(signal), float64(lifetime)
		result.Latest = append(result.Latest, e)
	}
	return result, rows.Err()
}
