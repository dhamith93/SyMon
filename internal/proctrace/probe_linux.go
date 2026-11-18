//go:build linux

package proctrace

import (
	"errors"
	"fmt"
	"os"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/features"
	"github.com/dhamith93/SyMon/internal/logger"
)

// probe checks what the BPF program needs before loading it, so the reason
// is clear when the host cannot run it. Features are asked of the kernel, as
// distro kernels backport them and the version says little.
func probe() error {
	if err := probeBTF(); err != nil {
		return err
	}
	if err := features.HaveMapType(ebpf.RingBuf); err != nil {
		if errors.Is(err, ebpf.ErrNotSupported) {
			return unavailable("the kernel has no BPF ring buffer, it needs 5.8 or newer")
		}
		if errors.Is(err, os.ErrPermission) {
			return unavailable("BPF needs root, or CAP_BPF and CAP_PERFMON")
		}
		return unavailable("cannot check for the BPF ring buffer: %v", err)
	}
	if !tracefsMounted() {
		return unavailable("tracefs is not mounted at /sys/kernel/tracing")
	}
	return nil
}

// probeBTF checks for the kernel's own type information, which CO-RE and
// fentry programs need
func probeBTF() error {
	if _, err := btf.LoadKernelSpec(); err != nil {
		return unavailable("the kernel has no BTF type information (%v)", err)
	}
	return nil
}

func tracefsMounted() bool {
	for _, dir := range []string{"/sys/kernel/tracing/events", "/sys/kernel/debug/tracing/events"} {
		if _, err := os.Stat(dir); err == nil {
			return true
		}
	}
	return false
}

// loadError explains why the kernel would not load a program. what names
// the tracer in the log, like "process tracing".
func loadError(what string, err error) error {
	var verifier *ebpf.VerifierError
	if errors.As(err, &verifier) {
		// the whole log, as it is the only way to find out what went wrong
		logger.Log("error", fmt.Sprintf("%s: the BPF verifier rejected the program:\n%+v", what, verifier))
		return unavailable("the kernel's BPF verifier rejected the program, see the log")
	}
	if errors.Is(err, os.ErrPermission) {
		return unavailable("BPF needs root, or CAP_BPF and CAP_PERFMON")
	}
	return unavailable("cannot load the BPF program: %v", err)
}
