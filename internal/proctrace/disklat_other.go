//go:build !linux

package proctrace

// DiskTracer only runs on Linux, like Tracer
type DiskTracer struct{}

func StartDiskLatency() (*DiskTracer, error) {
	return nil, unavailable("only on Linux")
}

func (t *DiskTracer) Drain() map[DiskOp][]uint64 { return nil }

func (t *DiskTracer) Close() error { return nil }
