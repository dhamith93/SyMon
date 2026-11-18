//go:build linux

package proctrace

import (
	"errors"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/dhamith93/SyMon/internal/logger"
)

// the operations the kernel side counts, the same numbers as OP_READ and
// OP_WRITE in disklat.bpf.c
const (
	opRead  uint32 = 0
	opWrite uint32 = 1
)

// DiskTracer counts how long each disk's reads and writes take until Close
type DiskTracer struct {
	objs disklatObjects
	link link.Link

	// the counters at the last Drain
	mu   sync.Mutex
	prev map[DiskOp][]uint64

	closeOnce sync.Once
}

// StartDiskLatency loads the latency counters and attaches them to request
// completion. The error wraps ErrUnavailable when the host cannot run them.
func StartDiskLatency() (*DiskTracer, error) {
	if err := probeBTF(); err != nil {
		return nil, err
	}
	// only kernels before 5.11 count BPF memory against memlock
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, unavailable("cannot raise the memlock limit: %v", err)
	}

	t := &DiskTracer{prev: map[DiskOp][]uint64{}}
	if err := loadDisklatObjects(&t.objs, nil); err != nil {
		return nil, loadError("disk latency", err)
	}
	l, err := link.AttachTracing(link.TracingOptions{Program: t.objs.BlockRqComplete})
	if err != nil {
		t.objs.Close()
		if errors.Is(err, ebpf.ErrNotSupported) {
			return nil, unavailable("the kernel cannot attach BTF tracepoints")
		}
		return nil, unavailable("cannot attach to block_rq_complete: %v", err)
	}
	t.link = l
	return t, nil
}

// Drain returns the IOs each disk finished since the last call, or since
// StartDiskLatency, in LatencyBuckets buckets. It returns nil when the
// counters cannot be read, and the next call covers that interval too.
func (t *DiskTracer) Drain() map[DiskOp][]uint64 {
	current, err := t.counts()
	if err != nil {
		logger.Log("error", "disk latency: cannot read the counters: "+err.Error())
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delta := latencyDelta(current, t.prev)
	t.prev = current
	return delta
}

// counts adds up each disk's buckets over the CPUs
func (t *DiskTracer) counts() (map[DiskOp][]uint64, error) {
	counts := map[DiskOp][]uint64{}
	var key disklatLatencyKey
	var perCPU []disklatLatencySlots
	iter := t.objs.Latency.Iterate()
	for iter.Next(&key, &perCPU) {
		if key.Op != opRead && key.Op != opWrite {
			continue
		}
		sum := make([]uint64, LatencyBuckets)
		for _, cpu := range perCPU {
			for i, c := range cpu.Slots {
				sum[i] += c
			}
		}
		op := DiskOp{Major: key.Dev >> 20, Minor: key.Dev & (1<<20 - 1), Write: key.Op == opWrite}
		counts[op] = sum
	}
	return counts, iter.Err()
}

// Close detaches the program and frees it
func (t *DiskTracer) Close() error {
	t.closeOnce.Do(func() {
		if t.link != nil {
			t.link.Close()
		}
		t.objs.Close()
	})
	return nil
}
