package proctrace

import (
	"sync"
	"time"
)

// window keeps the abnormal exits since the last Drain, up to max
type window struct {
	mu      sync.Mutex
	max     int
	from    time.Time
	exits   []Exit
	dropped int
}

func newWindow(max int, from time.Time) *window {
	return &window{max: max, from: from}
}

func (w *window) add(exit Exit) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.exits) >= w.max {
		w.dropped++
		return
	}
	w.exits = append(w.exits, exit)
}

// take returns what the window holds and starts a new one at now
func (w *window) take(now time.Time) (exits []Exit, dropped int, from time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	exits, dropped, from = w.exits, w.dropped, w.from
	w.exits, w.dropped, w.from = nil, 0, now
	return exits, dropped, from
}

// totals are the kernel's counters, added up over the CPUs. They only
// grow, so a window is the difference between two reads.
type totals [cntMax]uint64

func (t totals) since(prev totals) totals {
	var delta totals
	for i := range t {
		// a counter below its last read means the map was made again
		if t[i] >= prev[i] {
			delta[i] = t[i] - prev[i]
		} else {
			delta[i] = t[i]
		}
	}
	return delta
}

func summarize(from time.Time, to time.Time, delta totals, exits []Exit, dropped int) *Summary {
	return &Summary{
		From:         from.Unix(),
		To:           to.Unix(),
		Execs:        int(delta[cntExecs]),
		FailedExecs:  int(delta[cntFailedExecs]),
		Exits:        int(delta[cntExits]),
		NonZeroExits: int(delta[cntNonZero]),
		Signaled:     int(delta[cntSignaled]),
		Dropped:      int(delta[cntDrops]) + dropped,
		Abnormal:     exits,
	}
}
