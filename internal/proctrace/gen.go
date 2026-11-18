package proctrace

// compiles the BPF program and writes proctrace_bpfel.go/.o and
// proctrace_bpfeb.go/.o, which are committed
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -type exit_event proctrace bpf/proctrace.bpf.c -- -I./bpf -O2 -g -Wall -Wno-missing-declarations
