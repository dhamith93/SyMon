package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/tasks"
	"github.com/jackc/pgx/v5"
)

// ErrTaskBusy is returned when a host already has a run waiting or going.
// A host runs one task at a time.
var ErrTaskBusy = errors.New("this host is already running a task")

// What a task run can be
const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
	TaskTimedOut  = "timed_out"
	// TaskExpired is a run the agent did not pick up in time
	TaskExpired = "expired"
	// TaskLost is a run whose agent went away before it finished
	TaskLost = "lost"
)

const (
	// taskPickupWindow is how long a run waits for its agent. After that it
	// expires, so nothing runs by surprise when a host comes back.
	taskPickupWindow = 2 * time.Minute
	// taskLostAfter is how far past its timeout a run can go unreported
	taskLostAfter = time.Minute
	// TaskOutputLimit is how many characters of output a run keeps
	TaskOutputLimit = 256 << 10
	maxTaskError    = 500
	defaultTaskRuns = 50
	maxTaskRuns     = 200
)

// TaskRun is one time an admin asked a host to run a task
type TaskRun struct {
	ID          int64
	Host        string
	Task        string
	Command     string
	Status      string
	RequestedBy string
	RequestedAt time.Time
	// StartedAt and FinishedAt are zero until they happen
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	// Error says why a run could not start or finish
	Error string
	// Output is only filled in by TaskRun
	Output    string
	Truncated bool
}

// SaveHostTasks keeps the tasks a host's agent says its tasks file lists
func (s *Store) SaveHostTasks(ctx context.Context, host string, list []tasks.Task) error {
	if list == nil {
		list = []tasks.Task{}
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	// agents send the list with every ping, and it rarely changes
	_, err = s.pool.Exec(ctx, "UPDATE hosts SET tasks = $2 WHERE name = $1 AND tasks IS DISTINCT FROM $2", host, data)
	return err
}

// HostTasks returns the tasks a host offers
func (s *Store) HostTasks(ctx context.Context, host string) ([]tasks.Task, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, "SELECT tasks FROM hosts WHERE name = $1", host).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	list := []tasks.Task{}
	err = json.Unmarshal(data, &list)
	return list, err
}

// RequestTaskRun queues a run of one of the host's tasks for its agent to
// pick up. The command is copied from the host's list, and the agent runs
// it only when its tasks file still says the same. It returns ErrNotFound
// when the host does not list the task and ErrTaskBusy when the host has a
// run already.
func (s *Store) RequestTaskRun(ctx context.Context, host string, task string, by string) (TaskRun, error) {
	run := TaskRun{Host: host, Task: task, Status: TaskQueued, RequestedBy: by}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO task_runs (host_id, task, command, timeout_seconds, requested_by)
		SELECT h.id, t->>'Name', t->>'Command', (t->>'TimeoutSeconds')::integer, $3
		FROM hosts h, jsonb_array_elements(h.tasks) t
		WHERE h.name = $1 AND t->>'Name' = $2
		RETURNING id, command, requested_at`, host, task, by).Scan(&run.ID, &run.Command, &run.RequestedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return TaskRun{}, ErrNotFound
	case isUniqueViolation(err):
		return TaskRun{}, ErrTaskBusy
	}
	return run, err
}

// ClaimTaskRun hands a host's agent the run waiting for it, or nil. An
// agent asks while it is running nothing, so a run still marked as running
// was lost, most likely to an agent restart.
func (s *Store) ClaimTaskRun(ctx context.Context, host string) (*TaskRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE task_runs SET status = $2, finished_at = now(), error = 'the agent stopped before the task finished'
		WHERE status = $3 AND host_id = (SELECT id FROM hosts WHERE name = $1)`, host, TaskLost, TaskRunning)
	if err != nil {
		return nil, err
	}

	run := TaskRun{Host: host, Status: TaskRunning}
	err = tx.QueryRow(ctx, `
		UPDATE task_runs SET status = $2, started_at = now()
		WHERE status = $3 AND host_id = (SELECT id FROM hosts WHERE name = $1)
			AND requested_at > now() - make_interval(secs => $4)
		RETURNING id, task, command`, host, TaskRunning, TaskQueued, taskPickupWindow.Seconds()).Scan(&run.ID, &run.Task, &run.Command)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return &run, tx.Commit(ctx)
}

// AppendTaskOutput adds what a run printed since the last report. It
// returns ErrNotFound when the host has no such run going.
func (s *Store) AppendTaskOutput(ctx context.Context, host string, id int64, output string, truncated bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE task_runs SET
			output = output || left($3, greatest($4 - length(output), 0)),
			truncated = truncated OR $5 OR length(output) + length($3) > $4
		WHERE id = $2 AND status = $6 AND host_id = (SELECT id FROM hosts WHERE name = $1)`,
		host, id, cleanText(output), TaskOutputLimit, truncated, TaskRunning)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TaskResult is how a run ended
type TaskResult struct {
	// Output is what the run printed since the last report
	Output    string
	Truncated bool
	ExitCode  int
	TimedOut  bool
	// Error is set when the task could not be started or waited for
	Error string
}

func (result TaskResult) status() string {
	switch {
	case result.TimedOut:
		return TaskTimedOut
	case result.Error != "" || result.ExitCode != 0:
		return TaskFailed
	}
	return TaskSucceeded
}

// FinishTaskRun records how a run ended. It returns ErrNotFound when the
// host has no such run going.
func (s *Store) FinishTaskRun(ctx context.Context, host string, id int64, result TaskResult) error {
	message := []rune(cleanText(result.Error))
	if len(message) > maxTaskError {
		message = message[:maxTaskError]
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE task_runs SET
			output = output || left($3, greatest($4 - length(output), 0)),
			truncated = truncated OR $5 OR length(output) + length($3) > $4,
			status = $7, exit_code = $8, error = $9, finished_at = now()
		WHERE id = $2 AND status = $6 AND host_id = (SELECT id FROM hosts WHERE name = $1)`,
		host, id, cleanText(result.Output), TaskOutputLimit, result.Truncated, TaskRunning, result.status(), result.ExitCode, string(message))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// cleanText drops NUL characters, which a text column cannot hold
func cleanText(text string) string {
	return strings.ReplaceAll(text, "\x00", "")
}

const taskRunColumns = `r.id, h.name, r.task, r.command, r.status, r.requested_by, r.requested_at,
	r.started_at, r.finished_at, coalesce(r.exit_code, 0), r.error, r.truncated`

func scanTaskRun(row pgx.Row, run *TaskRun, more ...any) error {
	var started, finished *time.Time
	fields := []any{&run.ID, &run.Host, &run.Task, &run.Command, &run.Status, &run.RequestedBy, &run.RequestedAt,
		&started, &finished, &run.ExitCode, &run.Error, &run.Truncated}
	if err := row.Scan(append(fields, more...)...); err != nil {
		return err
	}
	if started != nil {
		run.StartedAt = *started
	}
	if finished != nil {
		run.FinishedAt = *finished
	}
	return nil
}

// TaskRuns returns a host's runs, newest first, without their output
func (s *Store) TaskRuns(ctx context.Context, host string, limit int) ([]TaskRun, error) {
	if _, err := s.hostID(ctx, host); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultTaskRuns
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskRunColumns+` FROM task_runs r JOIN hosts h ON h.id = r.host_id
		WHERE h.name = $1 ORDER BY r.id DESC LIMIT $2`, host, min(limit, maxTaskRuns))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []TaskRun{}
	for rows.Next() {
		var run TaskRun
		if err := scanTaskRun(rows, &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// TaskRun returns one run with its output
func (s *Store) TaskRun(ctx context.Context, id int64) (TaskRun, error) {
	var run TaskRun
	err := scanTaskRun(s.pool.QueryRow(ctx, `
		SELECT `+taskRunColumns+`, r.output FROM task_runs r JOIN hosts h ON h.id = r.host_id
		WHERE r.id = $1`, id), &run, &run.Output)
	if errors.Is(err, pgx.ErrNoRows) {
		return TaskRun{}, ErrNotFound
	}
	return run, err
}

// SweepTaskRuns ends the runs nobody will report on: those no agent picked
// up in time, and those still running well past their timeout because the
// host went away. It returns how many it ended.
func (s *Store) SweepTaskRuns(ctx context.Context) (int64, error) {
	expired, err := s.pool.Exec(ctx, `
		UPDATE task_runs SET status = $1, finished_at = now(), error = 'the agent did not pick the task up'
		WHERE status = $2 AND requested_at <= now() - make_interval(secs => $3)`,
		TaskExpired, TaskQueued, taskPickupWindow.Seconds())
	if err != nil {
		return 0, err
	}
	lost, err := s.pool.Exec(ctx, `
		UPDATE task_runs SET status = $1, finished_at = now(), error = 'the agent stopped reporting'
		WHERE status = $2 AND started_at + make_interval(secs => timeout_seconds + $3) < now()`,
		TaskLost, TaskRunning, int(taskLostAfter.Seconds()))
	if err != nil {
		return 0, err
	}
	return expired.RowsAffected() + lost.RowsAffected(), nil
}

// PurgeTaskRuns deletes finished runs asked for longer ago than keep
func (s *Store) PurgeTaskRuns(ctx context.Context, keep time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, "DELETE FROM task_runs WHERE requested_at < $1 AND status NOT IN ($2, $3)",
		time.Now().Add(-keep), TaskQueued, TaskRunning)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
