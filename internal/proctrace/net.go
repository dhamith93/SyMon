package proctrace

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// NetSummary is the network traffic of the host's programs between two
// Drain calls, loopback left out. Times are unix seconds.
type NetSummary struct {
	From int64
	To   int64
	// every program, the ones not listed too
	RxBytes uint64
	TxBytes uint64
	// the busiest programs, most bytes first
	Programs []ProgramTraffic `json:",omitempty"`
}

// ProgramTraffic is what all processes of one program received and sent
type ProgramTraffic struct {
	Name    string
	RxBytes uint64
	TxBytes uint64
}

// listedPrograms is how many programs a NetSummary names
const listedPrograms = 20

// byteCounts are received and sent bytes
type byteCounts struct {
	rx, tx uint64
}

// since returns the growth from prev. A counter below its last read means
// the kernel dropped the program from its map and has counted it again
// from zero since.
func (c byteCounts) since(prev byteCounts) byteCounts {
	delta := c
	if c.rx >= prev.rx {
		delta.rx = c.rx - prev.rx
	}
	if c.tx >= prev.tx {
		delta.tx = c.tx - prev.tx
	}
	return delta
}

// programName folds kernel threads that differ only by a number, like
// kworker/u8:2 and irq/45-eth0, into one name. A program's own name is the
// file it runs, which has no slash.
func programName(comm string) string {
	if i := strings.IndexByte(comm, '/'); i > 0 {
		return comm[:i]
	}
	return comm
}

// netDelta returns each program's growth since prev, the counters keyed by
// the kernel's comm
func netDelta(current map[string]byteCounts, prev map[string]byteCounts) map[string]byteCounts {
	delta := make(map[string]byteCounts, len(current))
	for comm, counts := range current {
		if grown := counts.since(prev[comm]); grown.rx > 0 || grown.tx > 0 {
			delta[comm] = grown
		}
	}
	return delta
}

// summarizeNet names the busiest programs. The total is read after the
// programs, so it holds everything they counted, but it is raised to their
// sum anyway so the unlisted rest is never negative.
func summarizeNet(from time.Time, to time.Time, delta map[string]byteCounts, total byteCounts) *NetSummary {
	byName := map[string]*ProgramTraffic{}
	var listed byteCounts
	for comm, counts := range delta {
		name := programName(comm)
		p := byName[name]
		if p == nil {
			p = &ProgramTraffic{Name: name}
			byName[name] = p
		}
		p.RxBytes += counts.rx
		p.TxBytes += counts.tx
		listed.rx += counts.rx
		listed.tx += counts.tx
	}

	programs := make([]ProgramTraffic, 0, len(byName))
	for _, p := range byName {
		programs = append(programs, *p)
	}
	slices.SortFunc(programs, func(a, b ProgramTraffic) int {
		return cmp.Or(cmp.Compare(b.RxBytes+b.TxBytes, a.RxBytes+a.TxBytes), cmp.Compare(a.Name, b.Name))
	})
	if len(programs) > listedPrograms {
		programs = programs[:listedPrograms]
	}

	return &NetSummary{
		From:     from.Unix(),
		To:       to.Unix(),
		RxBytes:  max(total.rx, listed.rx),
		TxBytes:  max(total.tx, listed.tx),
		Programs: programs,
	}
}
