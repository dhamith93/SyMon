package proctrace

import (
	"errors"
	"fmt"
	"os"
)

// Summary is the process activity between two Drain calls. Times are unix
// seconds.
type Summary struct {
	From int64
	To   int64
	// successful and failed execs. A shell trying each PATH entry fails
	// once per entry, so FailedExecs is often high and harmless.
	Execs       int
	FailedExecs int
	// every process that ended, and of those the ones with a non-zero
	// status or killed by a signal
	Exits        int
	NonZeroExits int
	Signaled     int
	// abnormal exits that were counted but are not in Abnormal, because the
	// ring buffer was full or there were more than Options.MaxExits
	Dropped  int
	Abnormal []Exit `json:",omitempty"`
}

// Exit is a process that ended with a non-zero status, a signal, or was
// chosen by the OOM killer
type Exit struct {
	Time   int64
	Name   string
	Pid    int
	Ppid   int
	Parent string
	Uid    int
	// exit status, 0 when a signal ended it
	Status int
	// the signal that ended it, 0 when it exited by itself
	Signal          int
	CoreDumped      bool `json:",omitempty"`
	OOM             bool `json:",omitempty"`
	LifetimeSeconds float64
}

// ErrUnavailable is wrapped with the reason when the host cannot run the
// tracer, like a kernel without BTF
var ErrUnavailable = errors.New("process tracing unavailable")

func unavailable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnavailable, fmt.Sprintf(format, args...))
}

// Options tune the tracer, zero values use the defaults
type Options struct {
	// abnormal exits kept between two Drain calls, more are only counted
	MaxExits int
	// size of the kernel ring buffer, rounded up to a power of two of at
	// least a page
	RingBufferBytes int
}

const (
	defaultMaxExits   = 50
	defaultRingBuffer = 256 * 1024
)

func (o Options) withDefaults() Options {
	if o.MaxExits <= 0 {
		o.MaxExits = defaultMaxExits
	}
	if o.RingBufferBytes <= 0 {
		o.RingBufferBytes = defaultRingBuffer
	}
	size := os.Getpagesize()
	for size < o.RingBufferBytes {
		size *= 2
	}
	o.RingBufferBytes = size
	return o
}
