package api

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/tasks"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Host tasks. A host lists the commands it is willing to run in its own
// tasks file, and its agent sends the list with each ping. An admin asks
// for a run on the dashboard. The agent waits for one in NextTask, runs it
// only when its file agrees, and reports back with ReportTask.

// TaskWaitTime is how long NextTask keeps an agent's call open when there
// is nothing to run. Agents give the call a longer deadline.
const TaskWaitTime = 25 * time.Second

// taskWaiters wakes the agents waiting in NextTask when a run is asked for
type taskWaiters struct {
	mu      sync.Mutex
	waiting map[string]chan struct{}
}

// wait returns a channel that closes when the host is woken
func (w *taskWaiters) wait(host string) <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.waiting == nil {
		w.waiting = map[string]chan struct{}{}
	}
	woken, ok := w.waiting[host]
	if !ok {
		woken = make(chan struct{})
		w.waiting[host] = woken
	}
	return woken
}

func (w *taskWaiters) wake(host string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if woken, ok := w.waiting[host]; ok {
		close(woken)
		delete(w.waiting, host)
	}
}

// saveTasks keeps the task list an agent sent with its ping. A list that
// breaks the rules of a tasks file did not come from an agent of ours, so
// the host then offers nothing.
func (s *Server) saveTasks(ctx context.Context, host string, sent []*TaskInfo) {
	list := make([]tasks.Task, 0, len(sent))
	for _, task := range sent {
		list = append(list, tasks.Task{
			Name:           task.Name,
			Description:    task.Description,
			Command:        task.Command,
			TimeoutSeconds: int(task.TimeoutSeconds),
		})
	}
	if err := tasks.Validate(list); err != nil {
		logger.Log("error", "ignoring the tasks "+host+" sent: "+err.Error())
		list = nil
	}
	if err := s.Store.SaveHostTasks(ctx, host, list); err != nil {
		logger.Log("error", "cannot save the tasks of "+host+": "+err.Error())
	}
}

// NextTask hands an agent the run an admin asked for. With nothing to run
// it keeps the call open for a while, so a run starts right when it is
// asked for. An empty reply means ask again.
func (s *Server) NextTask(ctx context.Context, in *TaskWait) (*TaskAssignment, error) {
	host := in.ServerName
	if authenticated, ok := agentHost(ctx); ok {
		host = authenticated
	}
	// before looking, so a run asked for in between is not missed
	woken := s.tasks.wait(host)
	run, err := s.Store.ClaimTaskRun(ctx, host)
	if err != nil {
		return nil, toStatus(err)
	}
	if run == nil {
		timer := time.NewTimer(TaskWaitTime)
		defer timer.Stop()
		select {
		case <-woken:
		case <-timer.C:
			return &TaskAssignment{}, nil
		case <-ctx.Done():
			// the agent hung up or gave up waiting, nothing went wrong here
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		if run, err = s.Store.ClaimTaskRun(ctx, host); err != nil {
			return nil, toStatus(err)
		}
		if run == nil {
			return &TaskAssignment{}, nil
		}
	}
	return &TaskAssignment{Id: run.ID, Task: run.Task, Command: run.Command}, nil
}

// ReportTask takes what a run printed and, with done, how it ended
func (s *Server) ReportTask(ctx context.Context, in *TaskProgress) (*Message, error) {
	host := in.ServerName
	if authenticated, ok := agentHost(ctx); ok {
		host = authenticated
	}
	var err error
	if in.Done {
		err = s.Store.FinishTaskRun(ctx, host, in.Id, store.TaskResult{
			Output:    in.Output,
			Truncated: in.Truncated,
			ExitCode:  int(in.ExitCode),
			TimedOut:  in.TimedOut,
			Error:     in.Error,
		})
	} else {
		err = s.Store.AppendTaskOutput(ctx, host, in.Id, in.Output, in.Truncated)
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "this run is not going any more")
	}
	if err != nil {
		return nil, toStatus(err)
	}
	if in.Done {
		logger.Log("info", "task run "+strconv.FormatInt(in.Id, 10)+" on "+host+" ended")
	}
	return &Message{Body: "ok"}, nil
}

// HostTasks returns the tasks a host offers
func (s *Server) HostTasks(ctx context.Context, in *HostRequest) (*TaskList, error) {
	list, err := s.Store.HostTasks(ctx, in.Host)
	if err != nil {
		return nil, toStatus(err)
	}
	response := &TaskList{}
	for _, task := range list {
		response.Tasks = append(response.Tasks, &TaskInfo{
			Name:           task.Name,
			Description:    task.Description,
			Command:        task.Command,
			TimeoutSeconds: int32(task.TimeoutSeconds),
		})
	}
	return response, nil
}

// RequestTaskRun queues a run of one of a host's tasks and wakes its agent
func (s *Server) RequestTaskRun(ctx context.Context, in *TaskRunRequest) (*TaskRunInfo, error) {
	if in.Host == "" || in.Task == "" || in.By == "" {
		return nil, status.Error(codes.InvalidArgument, "a run needs a host, a task and who asked")
	}
	run, err := s.Store.RequestTaskRun(ctx, in.Host, in.Task, in.By)
	if errors.Is(err, store.ErrNotFound) {
		return nil, status.Error(codes.NotFound, in.Host+" does not list a task named "+strconv.Quote(in.Task))
	}
	if err != nil {
		return nil, toStatus(err)
	}
	s.tasks.wake(in.Host)
	logger.Log("info", in.By+" asked "+in.Host+" to run task "+strconv.Quote(in.Task)+", run "+strconv.FormatInt(run.ID, 10))
	return taskRunInfo(run), nil
}

// TaskRuns returns a host's runs, newest first, without their output
func (s *Server) TaskRuns(ctx context.Context, in *TaskRunsRequest) (*TaskRunList, error) {
	runs, err := s.Store.TaskRuns(ctx, in.Host, int(in.Limit))
	if err != nil {
		return nil, toStatus(err)
	}
	response := &TaskRunList{}
	for _, run := range runs {
		response.Runs = append(response.Runs, taskRunInfo(run))
	}
	return response, nil
}

// TaskRun returns one run with its output
func (s *Server) TaskRun(ctx context.Context, in *TaskRunQuery) (*TaskRunInfo, error) {
	run, err := s.Store.TaskRun(ctx, in.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	return taskRunInfo(run), nil
}

func taskRunInfo(run store.TaskRun) *TaskRunInfo {
	return &TaskRunInfo{
		Id:          run.ID,
		Host:        run.Host,
		Task:        run.Task,
		Command:     run.Command,
		Status:      run.Status,
		RequestedBy: run.RequestedBy,
		RequestedAt: unix(run.RequestedAt),
		StartedAt:   unix(run.StartedAt),
		FinishedAt:  unix(run.FinishedAt),
		ExitCode:    int32(run.ExitCode),
		Error:       run.Error,
		Output:      run.Output,
		Truncated:   run.Truncated,
	}
}
