//go:build !linux

package proctrace

// Tracer only runs on Linux. This lets the agent and its tests build
// elsewhere, like on a Mac.
type Tracer struct{}

func Start(opts Options) (*Tracer, error) {
	return nil, unavailable("only on Linux")
}

func (t *Tracer) Drain() *Summary { return nil }

func (t *Tracer) Close() error { return nil }
