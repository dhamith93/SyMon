package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"
)

// how many programs ProcessNetwork returns at most
const networkProgramLimit = 50

// ProgramTraffic is what one program received and sent over a range, and
// its fastest agent interval, in bytes per second
type ProgramTraffic struct {
	Name    string
	RxBytes int64
	TxBytes int64
	RxPeak  float64
	TxPeak  float64
}

type ProcessNetworkResult struct {
	// every program, the ones not listed too
	RxBytes int64
	TxBytes int64
	// how much of the range the counters covered
	Seconds float64
	// the most bytes first
	Programs []ProgramTraffic
}

// ProcessNetwork returns what each program on a host received and sent over
// a range, from the same data as the proc_net charts so the two agree
func (s *Store) ProcessNetwork(ctx context.Context, host string, from time.Time, to time.Time) (ProcessNetworkResult, error) {
	if !to.After(from) {
		return ProcessNetworkResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return ProcessNetworkResult{}, err
	}

	source, _ := pickSource(seriesDefs["proc_net_rx"], from, to, time.Now(), defaultMaxPoints, s.retention)
	table, timeColumn := "process_network", "time"
	rxPeak, txPeak := "rx_bytes::float8 / nullif(seconds, 0)", "tx_bytes::float8 / nullif(seconds, 0)"
	if source != "raw" {
		table, timeColumn = "process_network_"+source, "bucket"
		rxPeak, txPeak = "rx_bps_max", "tx_bps_max"
	}
	// a snapshot has one row per listed program and one for the rest
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT name, sum(rx_bytes)::bigint, sum(tx_bytes)::bigint, coalesce(max(%[3]s), 0)::float8, coalesce(max(%[4]s), 0)::float8,
		       coalesce(sum(seconds) FILTER (WHERE name = ''), 0)::float8
		FROM %[1]s
		WHERE host_id = $1 AND %[2]s >= $2 AND %[2]s < $3
		GROUP BY name`, table, timeColumn, rxPeak, txPeak),
		hostID, from, to)
	if err != nil {
		return ProcessNetworkResult{}, err
	}
	defer rows.Close()

	result := ProcessNetworkResult{Programs: []ProgramTraffic{}}
	for rows.Next() {
		var p ProgramTraffic
		var seconds float64
		if err := rows.Scan(&p.Name, &p.RxBytes, &p.TxBytes, &p.RxPeak, &p.TxPeak, &seconds); err != nil {
			return ProcessNetworkResult{}, err
		}
		result.RxBytes += p.RxBytes
		result.TxBytes += p.TxBytes
		result.Seconds += seconds
		if p.Name != "" {
			result.Programs = append(result.Programs, p)
		}
	}
	if err := rows.Err(); err != nil {
		return ProcessNetworkResult{}, err
	}

	slices.SortFunc(result.Programs, func(a, b ProgramTraffic) int {
		return cmp.Or(cmp.Compare(b.RxBytes+b.TxBytes, a.RxBytes+a.TxBytes), cmp.Compare(a.Name, b.Name))
	})
	if len(result.Programs) > networkProgramLimit {
		result.Programs = result.Programs[:networkProgramLimit]
	}
	return result, nil
}
