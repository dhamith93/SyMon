// Package proctrace watches processes start and exit with eBPF.
//
// The kernel side counts every exec and exit, and sends only the exits that
// went wrong to user space: a non-zero status, a signal, or the OOM killer.
// Drain hands the counts and those exits over once per agent tick.
//
// It needs a Linux kernel with BTF (/sys/kernel/btf/vmlinux) and the BPF
// ring buffer (5.8 or newer, or a distro kernel with it backported), tracefs
// mounted, and root or CAP_BPF with CAP_PERFMON. Start returns an error
// wrapping ErrUnavailable with the reason when the host lacks any of them, so
// the agent can carry on without it.
//
// The BPF program is compiled with bpf2go and the results are committed, so
// building the agent needs no clang. After changing bpf/proctrace.bpf.c run
// make bpf, which needs clang, llvm and libbpf headers.
package proctrace
