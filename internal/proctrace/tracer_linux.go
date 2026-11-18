//go:build linux

package proctrace

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/dhamith93/SyMon/internal/logger"
)

// Tracer runs the BPF program until Close
type Tracer struct {
	objs   proctraceObjects
	links  []link.Link
	reader *ringbuf.Reader
	window *window
	wg     sync.WaitGroup

	// prev is the counters at the last Drain
	mu   sync.Mutex
	prev totals

	closeOnce sync.Once
}

// Start loads the BPF program and attaches it. The error wraps
// ErrUnavailable when the host cannot run it.
func Start(opts Options) (*Tracer, error) {
	opts = opts.withDefaults()
	if err := probe(); err != nil {
		return nil, err
	}
	// only kernels before 5.11 count BPF memory against memlock
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, unavailable("cannot raise the memlock limit: %v", err)
	}

	spec, err := loadProctrace()
	if err != nil {
		return nil, fmt.Errorf("cannot read the embedded BPF program: %w", err)
	}
	spec.Maps["events"].MaxEntries = uint32(opts.RingBufferBytes)

	t := &Tracer{window: newWindow(opts.MaxExits, time.Now())}
	if err := spec.LoadAndAssign(&t.objs, nil); err != nil {
		return nil, loadError(err)
	}

	// exits and execs are what the summary is about, so they must attach.
	// execveat is rare and the OOM mark only adds a flag.
	hooks := []struct {
		group, name string
		prog        *ebpf.Program
		required    bool
	}{
		{"sched", "sched_process_exit", t.objs.ProcessExit, true},
		{"syscalls", "sys_exit_execve", t.objs.ExitExecve, true},
		{"syscalls", "sys_exit_execveat", t.objs.ExitExecveat, false},
		{"oom", "mark_victim", t.objs.MarkVictim, false},
	}
	for _, hook := range hooks {
		l, err := link.Tracepoint(hook.group, hook.name, hook.prog, nil)
		if err != nil {
			if hook.required {
				t.closeBPF()
				return nil, unavailable("cannot attach to %s/%s: %v", hook.group, hook.name, err)
			}
			logger.Log("info", fmt.Sprintf("process tracing runs without %s/%s: %v", hook.group, hook.name, err))
			continue
		}
		t.links = append(t.links, l)
	}

	t.reader, err = ringbuf.NewReader(t.objs.Events)
	if err != nil {
		t.closeBPF()
		return nil, fmt.Errorf("cannot read the ring buffer: %w", err)
	}

	t.wg.Add(1)
	go t.readLoop()
	return t, nil
}

func (t *Tracer) readLoop() {
	defer t.wg.Done()
	var event proctraceExitEvent
	for {
		record, err := t.reader.Read()
		if errors.Is(err, ringbuf.ErrClosed) {
			return
		}
		if err != nil {
			logger.Log("error", "process tracing: "+err.Error())
			continue
		}
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.NativeEndian, &event); err != nil {
			logger.Log("error", "process tracing: cannot decode an exit: "+err.Error())
			continue
		}
		t.window.add(rawExit{
			pid:        event.Pid,
			ppid:       event.Ppid,
			uid:        event.Uid,
			exitCode:   event.ExitCode,
			lifetimeNs: event.LifetimeNs,
			comm:       event.Comm[:],
			parent:     event.ParentComm[:],
			oom:        event.Oom != 0,
		}.exit(time.Now()))
	}
}

// Drain returns the activity since the last call, or since Start
func (t *Tracer) Drain() *Summary {
	now := time.Now()
	current, err := t.counters()

	t.mu.Lock()
	var delta totals
	if err != nil {
		logger.Log("error", "process tracing: cannot read counters: "+err.Error())
	} else {
		delta = current.since(t.prev)
		t.prev = current
	}
	t.mu.Unlock()

	exits, dropped, from := t.window.take(now)
	return summarize(from, now, delta, exits, dropped)
}

// counters adds up each counter over the CPUs
func (t *Tracer) counters() (totals, error) {
	var sums totals
	var perCPU []uint64
	for slot := range sums {
		if err := t.objs.Counters.Lookup(uint32(slot), &perCPU); err != nil {
			return sums, err
		}
		for _, value := range perCPU {
			sums[slot] += value
		}
	}
	return sums, nil
}

// Close detaches the program and frees it
func (t *Tracer) Close() error {
	t.closeOnce.Do(func() {
		// closing the reader ends readLoop
		t.reader.Close()
		t.wg.Wait()
		t.closeBPF()
	})
	return nil
}

func (t *Tracer) closeBPF() {
	for _, l := range t.links {
		l.Close()
	}
	t.links = nil
	t.objs.Close()
}
