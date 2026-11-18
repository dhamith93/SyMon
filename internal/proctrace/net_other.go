//go:build !linux

package proctrace

// NetTracer only runs on Linux, like Tracer
type NetTracer struct{}

func StartNet() (*NetTracer, error) {
	return nil, unavailable("only on Linux")
}

func (t *NetTracer) Drain() *NetSummary { return nil }

func (t *NetTracer) Close() error { return nil }
