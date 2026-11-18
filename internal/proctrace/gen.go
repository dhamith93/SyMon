package proctrace

// compiles the BPF programs and writes proctrace_bpf{el,eb}.go/.o,
// procnet_bpf{el,eb}.go/.o and disklat_bpf{el,eb}.go/.o, which are committed
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -type exit_event proctrace bpf/proctrace.bpf.c -- -I./bpf -O2 -g -Wall -Wno-missing-declarations
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -type name_key -type traffic_counts procnet bpf/procnet.bpf.c -- -I./bpf -O2 -g -Wall -Wno-missing-declarations
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -type latency_key -type latency_slots disklat bpf/disklat.bpf.c -- -I./bpf -O2 -g -Wall -Wno-missing-declarations
