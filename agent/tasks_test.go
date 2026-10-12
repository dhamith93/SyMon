package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/tasks"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeTasks stands in for the collector. It hands out next once and keeps
// what the agent reported.
type fakeTasks struct {
	mu       sync.Mutex
	next     *api.TaskAssignment
	nextErr  error
	asked    int
	reports  []*api.TaskProgress
	failures int
	// onReport runs inside each report, while the task is still going
	onReport func()
}

func (f *fakeTasks) NextTask(ctx context.Context, in *api.TaskWait, opts ...grpc.CallOption) (*api.TaskAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	if f.nextErr != nil {
		return nil, f.nextErr
	}
	if f.next == nil {
		return &api.TaskAssignment{}, nil
	}
	next := f.next
	f.next = nil
	return next, nil
}

func (f *fakeTasks) ReportTask(ctx context.Context, in *api.TaskProgress, opts ...grpc.CallOption) (*api.Message, error) {
	if f.onReport != nil {
		f.onReport()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return nil, status.Error(codes.Unavailable, "collector away")
	}
	// the agent reuses the message, so keep a copy
	f.reports = append(f.reports, &api.TaskProgress{
		Id: in.Id, ServerName: in.ServerName, Output: in.Output, Done: in.Done,
		ExitCode: in.ExitCode, TimedOut: in.TimedOut, Error: in.Error, Truncated: in.Truncated,
	})
	return &api.Message{}, nil
}

// output is everything reported, and last the report that ended the run
func (f *fakeTasks) result(t *testing.T) (string, *api.TaskProgress) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reports) == 0 {
		t.Fatal("expected the run to be reported")
	}
	output := ""
	for i, report := range f.reports {
		output += report.Output
		if report.Done != (i == len(f.reports)-1) {
			t.Errorf("expected only the last report to end the run, report %d of %d has done %v", i+1, len(f.reports), report.Done)
		}
	}
	return output, f.reports[len(f.reports)-1]
}

// testRunner is a runner for a host whose tasks file lists the given tasks
func testRunner(t *testing.T, list ...tasks.Task) (*taskRunner, *fakeTasks) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	writeTasks(t, path, list...)
	fake := &fakeTasks{}
	runner := newTaskRunner(path, "web1", fake)
	runner.reportEvery = 20 * time.Millisecond
	runner.retryAfter = 10 * time.Millisecond
	return runner, fake
}

func writeTasks(t *testing.T, path string, list ...tasks.Task) {
	t.Helper()
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRunTask(t *testing.T) {
	task := tasks.Task{Name: "Say hello", Command: "echo out; echo err >&2; pwd"}
	runner, fake := testRunner(t, task)
	runner.run(&api.TaskAssignment{Id: 7, Task: task.Name, Command: task.Command})

	output, last := fake.result(t)
	if last.Id != 7 || last.ServerName != "web1" || last.ExitCode != 0 || last.TimedOut || last.Error != "" || last.Truncated {
		t.Errorf("unexpected end of the run %+v", last)
	}
	for _, want := range []string{"out\n", "err\n", "/\n"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in the output, got %q", want, output)
		}
	}
}

func TestRunTaskReportsWhileRunning(t *testing.T) {
	task := tasks.Task{Name: "Slow", Command: "echo one; sleep 0.3; echo two; exit 3"}
	runner, fake := testRunner(t, task)
	// the first report gets lost on the way, its output has to arrive later
	fake.failures = 1
	runner.run(&api.TaskAssignment{Id: 1, Task: task.Name, Command: task.Command})

	output, last := fake.result(t)
	if output != "one\ntwo\n" || last.ExitCode != 3 || last.Error != "" {
		t.Errorf("expected both lines and exit code 3, got %q %+v", output, last)
	}
	if len(fake.reports) < 2 {
		t.Errorf("expected output to be reported before the end, got %d reports", len(fake.reports))
	}
}

func TestRunTaskOnlyFromTheFile(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	task := tasks.Task{Name: "Touch", Command: "touch " + marker}
	runner, fake := testRunner(t, task)

	cases := []struct {
		assignment *api.TaskAssignment
		want       string
	}{
		// the collector cannot make up a task
		{&api.TaskAssignment{Id: 1, Task: "Something else", Command: "touch " + marker}, "does not list"},
		// and cannot run another command under a listed name
		{&api.TaskAssignment{Id: 2, Task: task.Name, Command: "touch " + marker + "; id"}, "changed on the host"},
	}
	for _, c := range cases {
		fake.reports = nil
		runner.run(c.assignment)
		output, last := fake.result(t)
		if !strings.Contains(last.Error, c.want) || last.ExitCode != -1 || output != "" {
			t.Errorf("expected run %d to be refused with %q, got %+v", c.assignment.Id, c.want, last)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("run %d: expected nothing to run", c.assignment.Id)
		}
	}

	// the file is read again for each run
	fake.reports = nil
	runner.run(&api.TaskAssignment{Id: 3, Task: task.Name, Command: task.Command})
	if _, last := fake.result(t); last.Error != "" || last.ExitCode != 0 {
		t.Errorf("expected the listed task to run, got %+v", last)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("expected the task to have run: %v", err)
	}
}

func TestRunTaskTimeout(t *testing.T) {
	// the shell starts a child and waits for it, both have to go
	task := tasks.Task{Name: "Hang", Command: "sleep 60 & echo $!; wait", TimeoutSeconds: 1}
	runner, fake := testRunner(t, task)
	started := time.Now()
	runner.run(&api.TaskAssignment{Id: 1, Task: task.Name, Command: task.Command})
	if took := time.Since(started); took > 4*time.Second {
		t.Errorf("expected the run to end at its timeout, it took %s", took)
	}

	output, last := fake.result(t)
	if !last.TimedOut || last.ExitCode != -1 {
		t.Errorf("expected a timed out run, got %+v", last)
	}
	child, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		t.Fatalf("expected the child's pid in the output, got %q", output)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(child, 0) == nil {
		if time.Now().After(deadline) {
			syscall.Kill(child, syscall.SIGKILL)
			t.Fatal("expected the shell's child to be killed too")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunTaskOutputLimit(t *testing.T) {
	task := tasks.Task{Name: "Chatty", Command: "head -c 300000 /dev/zero | tr '\\0' a; echo end"}
	runner, fake := testRunner(t, task)
	runner.run(&api.TaskAssignment{Id: 1, Task: task.Name, Command: task.Command})

	output, last := fake.result(t)
	if len(output) != taskOutputLimit || !last.Truncated || last.ExitCode != 0 || strings.Contains(output, "end") {
		t.Errorf("expected the output cut at the limit and the task to finish, got %d bytes, %+v", len(output), last.ExitCode)
	}
}

func TestRunTaskEnvironment(t *testing.T) {
	t.Setenv("SYMON_KEY", "the-collector-key")
	t.Setenv("SYMON_TEST_OTHER", "kept")
	t.Setenv("TASK_TEST_OTHER", "kept")
	task := tasks.Task{Name: "Env", Command: "env"}
	runner, fake := testRunner(t, task)
	runner.run(&api.TaskAssignment{Id: 1, Task: task.Name, Command: task.Command})

	output, _ := fake.result(t)
	if strings.Contains(output, "SYMON_") || strings.Contains(output, "the-collector-key") {
		t.Error("expected the agent's settings to be kept from the task")
	}
	if !strings.Contains(output, "TASK_TEST_OTHER=kept") || !strings.Contains(output, "PATH=") {
		t.Errorf("expected the rest of the environment, got %q", output)
	}
}

func TestRunTaskEndedBySignal(t *testing.T) {
	task := tasks.Task{Name: "Killed", Command: "kill -9 $$"}
	runner, fake := testRunner(t, task)
	runner.run(&api.TaskAssignment{Id: 1, Task: task.Name, Command: task.Command})
	if _, last := fake.result(t); last.ExitCode != -1 || !strings.Contains(last.Error, "killed") || last.TimedOut {
		t.Errorf("expected a run ended by a signal to say so, got %+v", last)
	}
}

// an update restarts the agent, so it has to wait for a running task
func TestWhenIdle(t *testing.T) {
	task := tasks.Task{Name: "Quick", Command: "true"}
	runner, fake := testRunner(t, task)
	duringRun := false
	fake.onReport = func() { runner.whenIdle(func() { duringRun = true }) }
	runner.run(&api.TaskAssignment{Id: 1, Task: task.Name, Command: task.Command})
	if duringRun {
		t.Error("expected nothing to run through whenIdle during a task")
	}
	after := false
	runner.whenIdle(func() { after = true })
	if !after {
		t.Error("expected whenIdle to run once the task is done")
	}
}

func TestTaskStep(t *testing.T) {
	task := tasks.Task{Name: "Quick", Command: "true"}
	runner, fake := testRunner(t, task)

	// nothing to run: ask again right away, the collector did the waiting
	if wait := runner.step(); wait != 0 || fake.asked != 1 || len(fake.reports) != 0 {
		t.Errorf("expected one question and no wait, got %s %d", wait, fake.asked)
	}
	fake.next = &api.TaskAssignment{Id: 9, Task: task.Name, Command: task.Command}
	if wait := runner.step(); wait != 0 {
		t.Errorf("expected no wait after a run, got %s", wait)
	}
	if _, last := fake.result(t); last.Id != 9 || last.ExitCode != 0 {
		t.Errorf("expected run 9 to be done, got %+v", last)
	}

	fake.nextErr = status.Error(codes.Unavailable, "collector away")
	if wait := runner.step(); wait != runner.retryAfter {
		t.Errorf("expected a short wait when the collector is away, got %s", wait)
	}
	// a collector from before tasks, for an agent with the shared key and one with its own
	for _, code := range []codes.Code{codes.Unimplemented, codes.PermissionDenied} {
		fake.nextErr = status.Error(code, "no")
		if wait := runner.step(); wait != taskOldCollector {
			t.Errorf("%s: expected a long wait for an old collector, got %s", code, wait)
		}
	}

	// a host without tasks does not ask at all
	asked := fake.asked
	for _, path := range []string{filepath.Join(t.TempDir(), "none.json"), ""} {
		runner.path = path
		if wait := runner.step(); wait != taskCheckEvery || fake.asked != asked {
			t.Errorf("expected a host without tasks to only look at its file, got %s", wait)
		}
	}
	// and neither does one with a broken file
	runner.path = filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(runner.path, []byte(`[{"Name": "a"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if wait := runner.step(); wait != taskCheckEvery || fake.asked != asked || len(runner.list()) != 0 {
		t.Errorf("expected a broken file to offer nothing, got %s", wait)
	}
}

func TestTaskList(t *testing.T) {
	runner, _ := testRunner(t,
		tasks.Task{Name: "Restart nginx", Description: "after a config change", Command: "systemctl restart nginx"},
		tasks.Task{Name: "Clean up", Command: "apt-get clean", TimeoutSeconds: 600})
	list := runner.list()
	if len(list) != 2 || list[0].Name != "Restart nginx" || list[0].Description != "after a config change" ||
		list[0].Command != "systemctl restart nginx" || list[0].TimeoutSeconds != 300 || list[1].TimeoutSeconds != 600 {
		t.Errorf("unexpected list %+v", list)
	}
}

func TestTaskOutput(t *testing.T) {
	output := &taskOutput{}
	// é is two bytes, and only the first has arrived
	output.Write([]byte("caf\xc3"))
	if text, _ := output.take(false); text != "caf" {
		t.Errorf("expected the half character to be held back, got %q", text)
	}
	output.Write([]byte("\xa9 \x00ok"))
	if text, _ := output.take(false); text != "é ok" {
		t.Errorf("expected the whole character and no NUL, got %q", text)
	}
	// at the end whatever is left goes out as valid text
	output.Write([]byte("end\xc3"))
	if text, dropped := output.take(true); text != "end�" || dropped {
		t.Errorf("expected the rest with a replacement character, got %q %v", text, dropped)
	}
	// bytes that are no text at all are not held back forever
	output.Write([]byte{0x80, 0x80, 0x80, 0x80, 0x80})
	if text, _ := output.take(false); text == "" {
		t.Error("expected binary output to be passed on")
	}
}
