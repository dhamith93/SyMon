package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/tasks"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Host tasks. This host's tasks file lists the commands an admin may start
// from the dashboard. The agent sends the list with its pings, waits at the
// collector for a run, and runs it only when the file still has that name
// with that command. Nothing the collector sends is ever run as it is.

const (
	// how often a host without tasks looks at its file again
	taskCheckEvery = 30 * time.Second
	taskRetryAfter = 15 * time.Second
	// how long to leave a collector from before tasks alone
	taskOldCollector = 5 * time.Minute
	taskReportEvery  = 2 * time.Second
	// tries for the report that ends a run
	taskEndTries = 5
	// a little longer than the collector keeps the call open
	taskWaitDeadline = api.TaskWaitTime + 15*time.Second
	// how much of a run's output is kept, in bytes
	taskOutputLimit = 256 << 10
	// how long a finished task's background children may keep its output open
	taskOutputWait = 5 * time.Second
)

// taskClient is the part of the collector client that tasks use
type taskClient interface {
	NextTask(ctx context.Context, in *api.TaskWait, opts ...grpc.CallOption) (*api.TaskAssignment, error)
	ReportTask(ctx context.Context, in *api.TaskProgress, opts ...grpc.CallOption) (*api.Message, error)
}

type taskRunner struct {
	path   string
	host   string
	client taskClient
	// reportEvery and retryAfter are shorter in tests
	reportEvery time.Duration
	retryAfter  time.Duration

	// working is held while a task runs
	working sync.Mutex

	mu           sync.Mutex
	loadError    string
	oldCollector bool
}

func newTaskRunner(path string, host string, client taskClient) *taskRunner {
	return &taskRunner{path: path, host: host, client: client, reportEvery: taskReportEvery, retryAfter: taskRetryAfter}
}

// load reads the tasks file. A file with a mistake in it offers no tasks,
// and the mistake is logged once, not on every look.
func (r *taskRunner) load() []tasks.Task {
	list, err := tasks.Load(r.path)
	message := ""
	if err != nil {
		message = err.Error()
	}
	r.mu.Lock()
	changed := message != r.loadError
	r.loadError = message
	r.mu.Unlock()
	if changed && message != "" {
		logger.Log("error", "this host offers no tasks: "+message)
	}
	return list
}

// list is what the ping tells the collector this host offers
func (r *taskRunner) list() []*api.TaskInfo {
	list := []*api.TaskInfo{}
	for _, task := range r.load() {
		list = append(list, &api.TaskInfo{
			Name:           task.Name,
			Description:    task.Description,
			Command:        task.Command,
			TimeoutSeconds: int32(task.TimeoutSeconds),
		})
	}
	return list
}

// whenIdle runs f unless a task is running, and keeps tasks from starting
// until f is done. An update restarts the agent, so it goes through here.
func (r *taskRunner) whenIdle(f func()) {
	if r.working.TryLock() {
		defer r.working.Unlock()
		f()
	}
}

func (r *taskRunner) loop() {
	for {
		time.Sleep(r.step())
	}
}

// step waits at the collector for one run and runs it. It returns how long
// to wait before the next step.
func (r *taskRunner) step() time.Duration {
	if len(r.load()) == 0 {
		return taskCheckEvery
	}
	ctx, cancel := context.WithTimeout(context.Background(), taskWaitDeadline)
	assignment, err := r.client.NextTask(ctx, &api.TaskWait{ServerName: r.host})
	cancel()
	switch status.Code(err) {
	case codes.OK:
	case codes.Unimplemented, codes.PermissionDenied:
		// a collector from before tasks does not know the call, or does
		// not let agents make it
		r.mu.Lock()
		known := r.oldCollector
		r.oldCollector = true
		r.mu.Unlock()
		if !known {
			logger.Log("error", "the collector is too old for tasks, update it to run them from the dashboard")
		}
		return taskOldCollector
	default:
		logger.Log("error", "cannot ask the collector for tasks: "+err.Error())
		return r.retryAfter
	}
	if assignment.Id != 0 {
		r.run(assignment)
	}
	return 0
}

// run does what an admin asked for, as long as this host's file agrees
func (r *taskRunner) run(assignment *api.TaskAssignment) {
	r.working.Lock()
	defer r.working.Unlock()

	run := "task " + strconv.Quote(assignment.Task) + ", run " + strconv.FormatInt(assignment.Id, 10)
	progress := &api.TaskProgress{Id: assignment.Id, ServerName: r.host}
	task, ok := tasks.Find(r.load(), assignment.Task)
	switch {
	case !ok:
		progress.Error = "this host's tasks file does not list the task any more"
	case task.Command != assignment.Command:
		progress.Error = "the task changed on the host, load the page again and check its command"
	}
	if progress.Error != "" {
		logger.Log("error", "not running "+run+": "+progress.Error)
		progress.Done = true
		progress.ExitCode = -1
		r.report(progress, taskEndTries)
		return
	}

	logger.Log("info", "running "+run+", asked for on the dashboard")
	ctx, cancel := context.WithTimeout(context.Background(), task.Timeout())
	defer cancel()
	output := &taskOutput{}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", task.Command)
	cmd.Dir = "/"
	cmd.Env = taskEnv()
	cmd.Stdout = output
	cmd.Stderr = output
	// its own process group, so a timeout also ends what the shell started
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = taskOutputWait

	if err := cmd.Start(); err != nil {
		progress.Done = true
		progress.ExitCode = -1
		progress.Error = "cannot start the task: " + err.Error()
		r.report(progress, taskEndTries)
		return
	}
	ended := make(chan error, 1)
	go func() { ended <- cmd.Wait() }()

	ticker := time.NewTicker(r.reportEvery)
	defer ticker.Stop()
	reporting := true
	for {
		select {
		case <-ticker.C:
			text, truncated := output.take(false)
			// output a failed report did not deliver goes with the next one
			progress.Output += text
			progress.Truncated = truncated
			if reporting && progress.Output != "" {
				switch status.Code(r.report(progress, 1)) {
				case codes.OK:
					progress.Output = ""
				case codes.NotFound:
					// the collector gave up on this run, let it finish quietly
					reporting = false
				}
			}
		case err := <-ended:
			text, truncated := output.take(true)
			progress.Output += text
			progress.Truncated = truncated
			progress.Done = true
			progress.ExitCode = -1
			if cmd.ProcessState != nil {
				progress.ExitCode = int32(cmd.ProcessState.ExitCode())
			}
			var exit *exec.ExitError
			switch {
			case err == nil, errors.Is(err, exec.ErrWaitDelay):
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				progress.TimedOut = true
			case errors.As(err, &exit) && exit.ExitCode() >= 0:
				// the exit code says it
			default:
				// ended by a signal, or could not be waited for
				progress.Error = err.Error()
			}
			logger.Log("info", run+" ended with exit code "+strconv.Itoa(int(progress.ExitCode)))
			if reporting {
				r.report(progress, taskEndTries)
			}
			return
		}
	}
}

// report sends progress to the collector, with more than one try when it
// matters. It returns the last error.
func (r *taskRunner) report(progress *api.TaskProgress, tries int) error {
	for try := 1; ; try++ {
		ctx, cancel := transport.Context()
		_, err := r.client.ReportTask(ctx, progress)
		cancel()
		if err == nil {
			return nil
		}
		logger.Log("error", "cannot report on task run "+strconv.FormatInt(progress.Id, 10)+": "+err.Error())
		if try >= tries || status.Code(err) == codes.NotFound {
			return err
		}
		time.Sleep(r.retryAfter)
	}
}

// taskEnv is the agent's environment without its own settings, which hold
// the collector key
func taskEnv() []string {
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "SYMON_") {
			env = append(env, entry)
		}
	}
	return env
}

// taskOutput collects what a task prints, up to taskOutputLimit
type taskOutput struct {
	mu      sync.Mutex
	pending []byte
	kept    int
	dropped bool
}

// Write never fails, so a task that prints too much carries on
func (o *taskOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	keep := min(len(data), taskOutputLimit-o.kept)
	if keep < len(data) {
		o.dropped = true
	}
	o.pending = append(o.pending, data[:keep]...)
	o.kept += keep
	return len(data), nil
}

// take returns what was printed since the last take as text, and whether
// output was dropped. Until the end it holds back a character that has
// only arrived in part.
func (o *taskOutput) take(end bool) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	whole := len(o.pending)
	if !end {
		whole = wholeCharacters(o.pending)
	}
	text := string(o.pending[:whole])
	o.pending = append([]byte{}, o.pending[whole:]...)
	// what goes into the database has to be text without NUL characters
	text = strings.ReplaceAll(strings.ToValidUTF8(text, "�"), "\x00", "")
	return text, o.dropped
}

// wholeCharacters returns how much of data can be sent without cutting
// its last UTF-8 character in half
func wholeCharacters(data []byte) int {
	for back := 1; back <= utf8.UTFMax && back <= len(data); back++ {
		start := len(data) - back
		if utf8.RuneStart(data[start]) {
			if utf8.FullRune(data[start:]) {
				return len(data)
			}
			return start
		}
	}
	return len(data)
}
