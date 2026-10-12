package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dhamith93/SyMon/internal/api"
)

// Host tasks: the commands a host lists in its own tasks file. Everyone
// logged in sees which tasks there are and how their runs went. Commands
// and output are for admins, and only admins start a run.

type hostTask struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Command        string `json:"command,omitempty"`
	TimeoutSeconds int32  `json:"timeoutSeconds"`
}

// taskRun leaves command and output out for viewers. Output is only ever
// in the answer for a single run.
type taskRun struct {
	ID          int64  `json:"id"`
	Host        string `json:"host"`
	Task        string `json:"task"`
	Command     string `json:"command,omitempty"`
	Status      string `json:"status"`
	RequestedBy string `json:"requestedBy"`
	RequestedAt int64  `json:"requestedAt"`
	StartedAt   int64  `json:"startedAt"`
	FinishedAt  int64  `json:"finishedAt"`
	ExitCode    int32  `json:"exitCode"`
	Error       string `json:"error"`
	Output      string `json:"output,omitempty"`
	Truncated   bool   `json:"truncated"`
}

func toTaskRun(run *api.TaskRunInfo, admin bool) taskRun {
	out := taskRun{
		ID:          run.Id,
		Host:        run.Host,
		Task:        run.Task,
		Status:      run.Status,
		RequestedBy: run.RequestedBy,
		RequestedAt: run.RequestedAt,
		StartedAt:   run.StartedAt,
		FinishedAt:  run.FinishedAt,
		ExitCode:    run.ExitCode,
		Error:       run.Error,
		Truncated:   run.Truncated,
	}
	if admin {
		out.Command = run.Command
		out.Output = run.Output
	}
	return out
}

func (s *server) isAdmin(r *http.Request) bool {
	session, err := s.sessionOf(r)
	return err == nil && session.role == "admin"
}

func (s *server) getHostTasks(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	response, err := s.collector.HostTasks(r.Context(), &api.HostRequest{Host: host})
	if err != nil {
		writeGRPCError(w, "tasks of "+host, err)
		return
	}
	admin := s.isAdmin(r)
	list := make([]hostTask, 0, len(response.Tasks))
	for _, task := range response.Tasks {
		item := hostTask{Name: task.Name, Description: task.Description, TimeoutSeconds: task.TimeoutSeconds}
		if admin {
			item.Command = task.Command
		}
		list = append(list, item)
	}
	writeJSON(w, map[string]any{"tasks": list})
}

func (s *server) getTaskRuns(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	response, err := s.collector.TaskRuns(r.Context(), &api.TaskRunsRequest{Host: host, Limit: int32(limit)})
	if err != nil {
		writeGRPCError(w, "task runs of "+host, err)
		return
	}
	admin := s.isAdmin(r)
	runs := make([]taskRun, 0, len(response.Runs))
	for _, run := range response.Runs {
		runs = append(runs, toTaskRun(run, admin))
	}
	writeJSON(w, map[string]any{"runs": runs})
}

// getTaskRun is one run with its output, for admins
func (s *server) getTaskRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "no such run")
		return
	}
	run, err := s.collector.TaskRun(r.Context(), &api.TaskRunQuery{Id: id})
	if err != nil {
		writeGRPCError(w, "task run "+r.PathValue("id"), err)
		return
	}
	writeJSON(w, toTaskRun(run, true))
}

// postTaskRun asks a host's agent to run one of the tasks the host lists.
// The task goes in the body, since names can hold any character.
func (s *server) postTaskRun(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the task as JSON")
		return
	}
	var body struct {
		Task string `json:"task"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.Task == "" {
		writeError(w, http.StatusBadRequest, "send the task to run")
		return
	}
	host := r.PathValue("host")
	session, _ := s.sessionOf(r)
	run, err := s.collector.RequestTaskRun(r.Context(), &api.TaskRunRequest{Host: host, Task: body.Task, By: session.user})
	if err != nil {
		writeGRPCError(w, "running a task on "+host, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toTaskRun(run, true))
}
