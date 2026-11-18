package proctrace

import (
	"bytes"
	"time"
)

// counter slots, the same numbers as the CNT_ defines in the BPF program
const (
	cntExecs = iota
	cntFailedExecs
	cntExits
	cntNonZero
	cntSignaled
	cntDrops
	cntMax
)

// splitExitCode reads the kernel's exit code the way waitpid does: the
// signal in the low 7 bits, core dumped in bit 7, the status in the next 8
func splitExitCode(code int32) (status int, signal int, coreDumped bool) {
	signal = int(code & 0x7f)
	coreDumped = code&0x80 != 0
	if signal == 0 {
		status = int((code >> 8) & 0xff)
	}
	return status, signal, coreDumped
}

// rawExit is the BPF program's exit_event without the generated type, so
// decoding can be tested on any OS
type rawExit struct {
	pid, ppid, uid uint32
	exitCode       int32
	lifetimeNs     uint64
	comm, parent   []byte
	oom            bool
}

func (r rawExit) exit(at time.Time) Exit {
	status, signal, core := splitExitCode(r.exitCode)
	return Exit{
		Time:            at.Unix(),
		Name:            cString(r.comm),
		Pid:             int(r.pid),
		Ppid:            int(r.ppid),
		Parent:          cString(r.parent),
		Uid:             int(r.uid),
		Status:          status,
		Signal:          signal,
		CoreDumped:      core,
		OOM:             r.oom,
		LifetimeSeconds: float64(r.lifetimeNs) / float64(time.Second),
	}
}

// cString is a NUL terminated C string as Go text
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
