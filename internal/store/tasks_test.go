package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/tasks"
)

func TestTaskRuns(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, host := range []string{"web1", "db1"} {
		if err := st.AddHost(ctx, host, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	list := []tasks.Task{
		{Name: "Restart nginx", Command: "systemctl restart nginx", TimeoutSeconds: 60},
		{Name: "Clean up", Description: "frees disk space", Command: "apt-get clean", TimeoutSeconds: 300},
	}
	// agents send the same list with every ping
	for range 2 {
		if err := st.SaveHostTasks(ctx, "web1", list); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := st.HostTasks(ctx, "web1"); err != nil || !reflect.DeepEqual(got, list) {
		t.Errorf("expected web1's tasks back, got %+v %v", got, err)
	}
	if got, err := st.HostTasks(ctx, "db1"); err != nil || len(got) != 0 {
		t.Errorf("expected no tasks for db1, got %+v %v", got, err)
	}
	if _, err := st.HostTasks(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown host, got %v", err)
	}
	if err := st.SaveHostTasks(ctx, "db1", nil); err != nil {
		t.Fatal(err)
	}
	if got, err := st.HostTasks(ctx, "db1"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("expected an empty list for db1, got %+v %v", got, err)
	}

	// only what the host lists can be asked for
	if _, err := st.RequestTaskRun(ctx, "web1", "rm -rf /", "alice"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for a task the host does not list, got %v", err)
	}
	if _, err := st.RequestTaskRun(ctx, "db1", "Restart nginx", "alice"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for another host's task, got %v", err)
	}
	run, err := st.RequestTaskRun(ctx, "web1", "Restart nginx", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == 0 || run.Status != TaskQueued || run.Command != "systemctl restart nginx" || run.RequestedAt.IsZero() {
		t.Errorf("unexpected queued run %+v", run)
	}
	if _, err := st.RequestTaskRun(ctx, "web1", "Clean up", "bob"); !errors.Is(err, ErrTaskBusy) {
		t.Errorf("expected ErrTaskBusy while a run waits, got %v", err)
	}

	// the run goes to web1's agent and nobody else
	if claimed, err := st.ClaimTaskRun(ctx, "db1"); err != nil || claimed != nil {
		t.Errorf("expected nothing for db1, got %+v %v", claimed, err)
	}
	claimed, err := st.ClaimTaskRun(ctx, "web1")
	if err != nil || claimed == nil || claimed.ID != run.ID || claimed.Task != "Restart nginx" || claimed.Command != run.Command {
		t.Fatalf("expected web1 to get its run, got %+v %v", claimed, err)
	}
	if _, err := st.RequestTaskRun(ctx, "web1", "Clean up", "bob"); !errors.Is(err, ErrTaskBusy) {
		t.Errorf("expected ErrTaskBusy while a run is going, got %v", err)
	}
	if err := st.AppendTaskOutput(ctx, "web1", run.ID, "hello\x00\n", false); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendTaskOutput(ctx, "db1", run.ID, "not mine", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for another host's run, got %v", err)
	}
	if err := st.FinishTaskRun(ctx, "db1", run.ID, TaskResult{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound finishing another host's run, got %v", err)
	}
	if err := st.FinishTaskRun(ctx, "web1", run.ID, TaskResult{Output: "done\n"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.TaskRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != TaskSucceeded || got.Output != "hello\ndone\n" || got.Host != "web1" || got.RequestedBy != "alice" ||
		got.StartedAt.IsZero() || got.FinishedAt.IsZero() || got.ExitCode != 0 || got.Truncated {
		t.Errorf("unexpected finished run %+v", got)
	}
	if err := st.FinishTaskRun(ctx, "web1", run.ID, TaskResult{ExitCode: 1}); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound finishing a run twice, got %v", err)
	}
	if err := st.AppendTaskOutput(ctx, "web1", run.ID, "late", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for output after the end, got %v", err)
	}

	// start asks for a run and hands it to the agent
	start := func(task string) int64 {
		t.Helper()
		run, err := st.RequestTaskRun(ctx, "web1", task, "bob")
		if err != nil {
			t.Fatal(err)
		}
		claimed, err := st.ClaimTaskRun(ctx, "web1")
		if err != nil || claimed == nil || claimed.ID != run.ID {
			t.Fatalf("expected run %d to be claimed, got %+v %v", run.ID, claimed, err)
		}
		return run.ID
	}
	status := func(id int64) TaskRun {
		t.Helper()
		run, err := st.TaskRun(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return run
	}

	endings := []struct {
		result TaskResult
		want   string
	}{
		{TaskResult{ExitCode: 3}, TaskFailed},
		{TaskResult{ExitCode: -1, TimedOut: true}, TaskTimedOut},
		{TaskResult{ExitCode: -1, Error: "the task changed on the host"}, TaskFailed},
	}
	for _, ending := range endings {
		id := start("Clean up")
		if err := st.FinishTaskRun(ctx, "web1", id, ending.result); err != nil {
			t.Fatal(err)
		}
		if got := status(id); got.Status != ending.want || got.ExitCode != ending.result.ExitCode || got.Error != ending.result.Error {
			t.Errorf("expected %s for %+v, got %+v", ending.want, ending.result, got)
		}
	}

	// output past the limit is dropped
	id := start("Clean up")
	if err := st.AppendTaskOutput(ctx, "web1", id, strings.Repeat("a", TaskOutputLimit-10), false); err != nil {
		t.Fatal(err)
	}
	if got := status(id); got.Truncated {
		t.Error("expected output under the limit not to be cut off")
	}
	if err := st.AppendTaskOutput(ctx, "web1", id, strings.Repeat("b", 100), false); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishTaskRun(ctx, "web1", id, TaskResult{Output: "c"}); err != nil {
		t.Fatal(err)
	}
	if got := status(id); len(got.Output) != TaskOutputLimit || !strings.HasSuffix(got.Output, "abbbbbbbbbb") || !got.Truncated {
		t.Errorf("expected the output cut at the limit, got %d characters, truncated %v", len(got.Output), got.Truncated)
	}
	// the agent can say it dropped output itself
	id = start("Clean up")
	if err := st.FinishTaskRun(ctx, "web1", id, TaskResult{Output: "short", Truncated: true}); err != nil {
		t.Fatal(err)
	}
	if got := status(id); !got.Truncated || got.Output != "short" {
		t.Errorf("expected the agent's cut off flag to be kept, got %+v", got)
	}

	// a run no agent picks up expires, so it cannot start later by surprise
	waiting, err := st.RequestTaskRun(ctx, "web1", "Restart nginx", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if ended, err := st.SweepTaskRuns(ctx); err != nil || ended != 0 {
		t.Errorf("expected a fresh run to stay, got %d %v", ended, err)
	}
	if _, err := st.pool.Exec(ctx, "UPDATE task_runs SET requested_at = now() - INTERVAL '3 minutes' WHERE id = $1", waiting.ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimTaskRun(ctx, "web1"); err != nil || claimed != nil {
		t.Errorf("expected an old run not to be handed out, got %+v %v", claimed, err)
	}
	if ended, err := st.SweepTaskRuns(ctx); err != nil || ended != 1 {
		t.Errorf("expected the old run to expire, got %d %v", ended, err)
	}
	if got := status(waiting.ID); got.Status != TaskExpired || got.FinishedAt.IsZero() || got.Error == "" || !got.StartedAt.IsZero() {
		t.Errorf("unexpected expired run %+v", got)
	}

	// an agent asking for work is running nothing, so its run was lost
	id = start("Restart nginx")
	if claimed, err := st.ClaimTaskRun(ctx, "web1"); err != nil || claimed != nil {
		t.Errorf("expected nothing new to claim, got %+v %v", claimed, err)
	}
	if got := status(id); got.Status != TaskLost || got.FinishedAt.IsZero() {
		t.Errorf("expected the run to be lost, got %+v", got)
	}

	// a host that went away mid run: lost once the timeout is well over
	id = start("Restart nginx")
	if _, err := st.pool.Exec(ctx, "UPDATE task_runs SET started_at = now() - INTERVAL '90 seconds' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if ended, err := st.SweepTaskRuns(ctx); err != nil || ended != 0 {
		t.Errorf("expected a run just past its timeout to stay, got %d %v", ended, err)
	}
	if _, err := st.pool.Exec(ctx, "UPDATE task_runs SET started_at = now() - INTERVAL '3 minutes' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if ended, err := st.SweepTaskRuns(ctx); err != nil || ended != 1 {
		t.Errorf("expected the silent run to be ended, got %d %v", ended, err)
	}
	if got := status(id); got.Status != TaskLost {
		t.Errorf("expected the silent run to be lost, got %+v", got)
	}

	// the history is newest first and leaves the output out
	runs, err := st.TaskRuns(ctx, "web1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 9 || runs[0].ID != id || runs[8].ID != run.ID {
		t.Fatalf("expected 9 runs, newest first, got %d", len(runs))
	}
	for _, listed := range runs {
		if listed.Output != "" {
			t.Errorf("expected no output in the history, run %d has some", listed.ID)
		}
	}
	if runs, err := st.TaskRuns(ctx, "web1", 2); err != nil || len(runs) != 2 {
		t.Errorf("expected the limit to apply, got %d %v", len(runs), err)
	}
	if runs, err := st.TaskRuns(ctx, "db1", 0); err != nil || len(runs) != 0 {
		t.Errorf("expected no runs for db1, got %d %v", len(runs), err)
	}
	if _, err := st.TaskRuns(ctx, "ghost", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown host, got %v", err)
	}
	if _, err := st.TaskRun(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown run, got %v", err)
	}

	// old runs are purged, but never one that is still going
	going := start("Clean up")
	if _, err := st.pool.Exec(ctx, "UPDATE task_runs SET requested_at = now() - INTERVAL '100 days' WHERE id IN ($1, $2)", run.ID, going); err != nil {
		t.Fatal(err)
	}
	if purged, err := st.PurgeTaskRuns(ctx, 90*24*time.Hour); err != nil || purged != 1 {
		t.Errorf("expected one old run purged, got %d %v", purged, err)
	}
	if _, err := st.TaskRun(ctx, run.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected the purged run to be gone, got %v", err)
	}

	// a removed host takes its runs with it
	if err := st.RemoveHost(ctx, "web1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.TaskRun(ctx, going); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected a removed host's runs to be gone, got %v", err)
	}
}
