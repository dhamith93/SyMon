// Package tasks reads a host's tasks file: the commands the host's owner
// lets admins start from the dashboard. Nothing that is not in the file
// can be run that way.
package tasks

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dhamith93/SyMon/internal/strictjson"
)

const (
	defaultTimeoutSeconds = 300
	maxTimeoutSeconds     = 24 * 60 * 60
	maxTasks              = 50
	maxNameLength         = 64
	maxDescriptionLength  = 200
	maxCommandLength      = 4096
	maxFileSize           = 1 << 20
)

// Task is one command a host offers
type Task struct {
	Name        string
	Description string `json:",omitempty"`
	// Command is run with /bin/sh -c
	Command string
	// TimeoutSeconds is how long a run may take, 300 when left out
	TimeoutSeconds int
}

func (task Task) Timeout() time.Duration {
	return time.Duration(task.TimeoutSeconds) * time.Second
}

// Load reads a tasks file. A host without one offers no tasks. Any mistake
// in the file is an error, and so is a file other users could have written,
// because whoever writes it runs commands as the agent.
func Load(path string) ([]Task, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := checkOwner(info); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("%s: too big for a tasks file", path)
	}
	tasks, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return tasks, nil
}

func checkOwner(info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("group or others can write to it, run chmod go-w on it")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 && int(stat.Uid) != os.Geteuid() {
		return errors.New("it has to belong to root or to the user the agent runs as")
	}
	return nil
}

// Parse reads a JSON list of tasks. Unknown fields are errors.
func Parse(data []byte) ([]Task, error) {
	var tasks []Task
	if err := strictjson.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	if err := Validate(tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// Validate checks a list of tasks and fills in the default timeout. The
// collector runs it too, on the list an agent sends.
func Validate(tasks []Task) error {
	if len(tasks) > maxTasks {
		return fmt.Errorf("a host can list at most %d tasks", maxTasks)
	}
	names := map[string]bool{}
	for i := range tasks {
		task := &tasks[i]
		if err := task.validate(); err != nil {
			return fmt.Errorf("task %d %q: %w", i+1, task.Name, err)
		}
		if names[task.Name] {
			return fmt.Errorf("task %d: there is already a task named %q", i+1, task.Name)
		}
		names[task.Name] = true
	}
	return nil
}

func (task *Task) validate() error {
	task.Name = strings.TrimSpace(task.Name)
	task.Description = strings.TrimSpace(task.Description)
	if task.TimeoutSeconds == 0 {
		task.TimeoutSeconds = defaultTimeoutSeconds
	}
	switch {
	case task.Name == "":
		return errors.New("a task needs a Name")
	case utf8.RuneCountInString(task.Name) > maxNameLength:
		return fmt.Errorf("a Name can be at most %d characters", maxNameLength)
	case strings.ContainsFunc(task.Name, unicode.IsControl):
		return errors.New("a Name is one line of text")
	case utf8.RuneCountInString(task.Description) > maxDescriptionLength:
		return fmt.Errorf("a Description can be at most %d characters", maxDescriptionLength)
	case strings.ContainsFunc(task.Description, unicode.IsControl):
		return errors.New("a Description is one line of text")
	case strings.TrimSpace(task.Command) == "":
		return errors.New("a task needs a Command")
	case len(task.Command) > maxCommandLength:
		return fmt.Errorf("a Command can be at most %d bytes, put longer ones in a script", maxCommandLength)
	case strings.ContainsRune(task.Command, 0):
		return errors.New("a Command cannot hold a NUL character")
	case task.TimeoutSeconds < 0 || task.TimeoutSeconds > maxTimeoutSeconds:
		return fmt.Errorf("TimeoutSeconds goes from 1 to %d", maxTimeoutSeconds)
	}
	return nil
}

// Find returns the task with that name
func Find(tasks []Task, name string) (Task, bool) {
	for _, task := range tasks {
		if task.Name == name {
			return task, true
		}
	}
	return Task{}, false
}
