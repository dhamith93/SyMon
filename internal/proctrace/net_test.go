package proctrace

import (
	"fmt"
	"testing"
	"time"
)

func TestProgramName(t *testing.T) {
	for comm, want := range map[string]string{
		"nginx":           "nginx",
		"kworker/u8:2":    "kworker",
		"irq/45-eth0":     "irq",
		"tokio-runtime-w": "tokio-runtime-w",
		"/weird":          "/weird",
	} {
		if got := programName(comm); got != want {
			t.Errorf("programName(%q) = %q, want %q", comm, got, want)
		}
	}
}

func TestNetDelta(t *testing.T) {
	prev := map[string]byteCounts{
		"curl":  {rx: 100, tx: 10},
		"nginx": {rx: 5000, tx: 90000},
		"sshd":  {rx: 70, tx: 80},
		// gone from the map since, so it adds nothing
		"wget": {rx: 1, tx: 1},
	}
	current := map[string]byteCounts{
		"curl":  {rx: 400, tx: 10},
		"nginx": {rx: 200, tx: 300},
		"sshd":  {rx: 70, tx: 80},
		"dig":   {rx: 60, tx: 40},
	}
	delta := netDelta(current, prev)
	want := map[string]byteCounts{
		"curl": {rx: 300},
		// dropped by the kernel and counted from zero again
		"nginx": {rx: 200, tx: 300},
		"dig":   {rx: 60, tx: 40},
	}
	if len(delta) != len(want) {
		t.Fatalf("unexpected delta %v, want %v", delta, want)
	}
	for comm, counts := range want {
		if delta[comm] != counts {
			t.Errorf("%s grew by %+v, want %+v", comm, delta[comm], counts)
		}
	}
}

func TestSummarizeNet(t *testing.T) {
	from := time.Unix(1000, 0)
	to := from.Add(15 * time.Second)
	delta := map[string]byteCounts{
		"nginx":        {rx: 1000, tx: 9000},
		"kworker/u8:2": {rx: 10, tx: 0},
		"kworker/1:0":  {rx: 5, tx: 5},
	}
	// 30 programs that each moved less than nginx and kworker, by name
	for i := range 30 {
		delta[fmt.Sprintf("job%02d", i)] = byteCounts{rx: 1}
	}

	summary := summarizeNet(from, to, delta, byteCounts{rx: 50000, tx: 9100})
	if summary.From != 1000 || summary.To != 1015 || summary.RxBytes != 50000 || summary.TxBytes != 9100 {
		t.Errorf("unexpected summary %+v", summary)
	}
	if len(summary.Programs) != listedPrograms {
		t.Fatalf("expected %d programs, got %d", listedPrograms, len(summary.Programs))
	}
	if p := summary.Programs[0]; p != (ProgramTraffic{Name: "nginx", RxBytes: 1000, TxBytes: 9000}) {
		t.Errorf("expected nginx first, got %+v", p)
	}
	if p := summary.Programs[1]; p != (ProgramTraffic{Name: "kworker", RxBytes: 15, TxBytes: 5}) {
		t.Errorf("expected the kernel workers as one program, got %+v", p)
	}
	if p := summary.Programs[2]; p.Name != "job00" {
		t.Errorf("expected ties in name order, got %+v", p)
	}

	// a total read a moment before a program counted more is raised
	summary = summarizeNet(from, to, map[string]byteCounts{"curl": {rx: 700, tx: 3}}, byteCounts{rx: 500, tx: 3})
	if summary.RxBytes != 700 || summary.TxBytes != 3 {
		t.Errorf("expected the total to cover the programs, got %+v", summary)
	}

	// a quiet interval still says it was traced
	summary = summarizeNet(from, to, nil, byteCounts{})
	if summary == nil || len(summary.Programs) != 0 || summary.RxBytes != 0 || summary.To != 1015 {
		t.Errorf("unexpected quiet summary %+v", summary)
	}
}
