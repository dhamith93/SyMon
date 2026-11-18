//go:build linux

package proctrace

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/dhamith93/SyMon/internal/logger"
)

// total slots, the same numbers as TOTAL_RX and TOTAL_TX in procnet.bpf.c
const (
	totalRx uint32 = 0
	totalTx uint32 = 1
)

// NetTracer counts each program's network traffic until Close
type NetTracer struct {
	maps  procnetMaps
	progs []*ebpf.Program
	links []link.Link

	// the counters at the last Drain
	mu        sync.Mutex
	from      time.Time
	prev      map[string]byteCounts
	prevTotal byteCounts

	closeOnce sync.Once
}

// netHooks are the kernel functions counted. TCP must attach, as it carries
// most traffic. UDP may be missing, like udpv6 without IPv6, and
// tcp_sendpage is gone since 6.5, where tcp_sendmsg covers sendfile.
var netHooks = []struct {
	prog     string
	required bool
	quiet    bool
}{
	{prog: "tcp_sendmsg_exit", required: true},
	{prog: "tcp_recvmsg_exit", required: true},
	{prog: "udp_sendmsg_exit"},
	{prog: "udp_recvmsg_exit"},
	{prog: "udpv6_sendmsg_exit"},
	{prog: "udpv6_recvmsg_exit"},
	{prog: "tcp_sendpage_exit", quiet: true},
}

// StartNet loads the network counters and attaches them. The error wraps
// ErrUnavailable when the host cannot run them.
func StartNet() (*NetTracer, error) {
	if err := probeBTF(); err != nil {
		return nil, err
	}
	// only kernels before 5.11 count BPF memory against memlock
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, unavailable("cannot raise the memlock limit: %v", err)
	}

	spec, err := loadProcnet()
	if err != nil {
		return nil, fmt.Errorf("cannot read the embedded BPF program: %w", err)
	}
	t := &NetTracer{from: time.Now(), prev: map[string]byteCounts{}}
	if err := spec.LoadAndAssign(&t.maps, nil); err != nil {
		return nil, loadError("process network", err)
	}

	// each program loads on its own, so a kernel function that is missing
	// only costs its own hook
	cache := btf.NewCache()
	maps := map[string]*ebpf.Map{"traffic": t.maps.Traffic, "totals": t.maps.Totals}
	for _, hook := range netHooks {
		err := t.attach(spec, hook.prog, maps, cache)
		switch {
		case err == nil:
		case hook.required:
			t.Close()
			return nil, err
		case !hook.quiet:
			logger.Log("info", fmt.Sprintf("process network runs without %s: %v", strings.TrimSuffix(hook.prog, "_exit"), err))
		}
	}
	return t, nil
}

func (t *NetTracer) attach(spec *ebpf.CollectionSpec, name string, maps map[string]*ebpf.Map, cache *btf.Cache) error {
	one := spec.Copy()
	for other := range one.Programs {
		if other != name {
			delete(one.Programs, other)
		}
	}
	coll, err := ebpf.NewCollectionWithOptions(one, ebpf.CollectionOptions{MapReplacements: maps, Cache: cache})
	if err != nil {
		return loadError("process network", err)
	}
	prog := coll.DetachProgram(name)
	coll.Close()

	l, err := link.AttachTracing(link.TracingOptions{Program: prog})
	if err != nil {
		prog.Close()
		if errors.Is(err, ebpf.ErrNotSupported) {
			return unavailable("the kernel cannot attach fentry programs, arm64 needs 6.0 or newer")
		}
		return unavailable("cannot attach to %s: %v", spec.Programs[name].AttachTo, err)
	}
	t.progs = append(t.progs, prog)
	t.links = append(t.links, l)
	return nil
}

// Drain returns the traffic since the last call, or since StartNet. It
// returns nil when the counters cannot be read, and the next call covers
// that interval too.
func (t *NetTracer) Drain() *NetSummary {
	now := time.Now()
	// the programs first, so the total holds everything they counted
	current, err := t.programs()
	if err != nil {
		logger.Log("error", "process network: cannot read the counters: "+err.Error())
		return nil
	}
	total, err := t.totals()
	if err != nil {
		logger.Log("error", "process network: cannot read the totals: "+err.Error())
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	delta := netDelta(current, t.prev)
	grown := total.since(t.prevTotal)
	// kept when missing from a read, so a program the walk skipped is not
	// counted twice when it shows again. A map of names the kernel no longer
	// has starts over once it is twice the kernel's.
	if len(t.prev) > 2*int(t.maps.Traffic.MaxEntries()) {
		t.prev = map[string]byteCounts{}
	}
	for comm, counts := range current {
		t.prev[comm] = counts
	}
	t.prevTotal = total
	from := t.from
	t.from = now
	return summarizeNet(from, now, delta, grown)
}

// programs adds up each program's counters over the CPUs
func (t *NetTracer) programs() (map[string]byteCounts, error) {
	counts := map[string]byteCounts{}
	var key procnetNameKey
	var perCPU []procnetTrafficCounts
	iter := t.maps.Traffic.Iterate()
	for iter.Next(&key, &perCPU) {
		var c byteCounts
		for _, v := range perCPU {
			c.rx += v.RxBytes
			c.tx += v.TxBytes
		}
		counts[cString(key.Comm[:])] = c
	}
	return counts, iter.Err()
}

func (t *NetTracer) totals() (byteCounts, error) {
	var c byteCounts
	for slot, sum := range map[uint32]*uint64{totalRx: &c.rx, totalTx: &c.tx} {
		var perCPU []uint64
		if err := t.maps.Totals.Lookup(slot, &perCPU); err != nil {
			return c, err
		}
		for _, v := range perCPU {
			*sum += v
		}
	}
	return c, nil
}

// Close detaches the programs and frees them
func (t *NetTracer) Close() error {
	t.closeOnce.Do(func() {
		for _, l := range t.links {
			l.Close()
		}
		for _, p := range t.progs {
			p.Close()
		}
		t.links, t.progs = nil, nil
		t.maps.Close()
	})
	return nil
}
