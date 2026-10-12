package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tasks, err := Parse([]byte(`[
		{"Name": " Restart nginx ", "Description": "after a config change", "Command": "systemctl restart nginx"},
		{"Name": "Clean up", "Command": "apt-get clean\njournalctl --vacuum-time=7d", "TimeoutSeconds": 600}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].Name != "Restart nginx" || tasks[0].Timeout() != 5*time.Minute || tasks[1].TimeoutSeconds != 600 {
		t.Errorf("unexpected tasks %+v", tasks)
	}
	if task, ok := Find(tasks, "Clean up"); !ok || !strings.HasPrefix(task.Command, "apt-get clean") {
		t.Errorf("expected to find the second task, got %+v %v", task, ok)
	}
	if _, ok := Find(tasks, "clean up"); ok {
		t.Error("expected names to match exactly")
	}
}

func TestParseRefusesMistakes(t *testing.T) {
	cases := []struct {
		name string
		file string
		want string
	}{
		{"unknown field", `[{"Name": "a", "Comand": "true"}]`, `unknown field "Comand"`},
		{"syntax", "[\n  {\"Name\": \"a\" \"Command\": \"true\"}\n]", "line 2, column 16"},
		{"no name", `[{"Command": "true"}]`, "needs a Name"},
		{"long name", `[{"Name": "` + strings.Repeat("a", 65) + `", "Command": "true"}]`, "at most 64"},
		{"name with a newline", `[{"Name": "a\nb", "Command": "true"}]`, "one line"},
		{"no command", `[{"Name": "a", "Command": "  "}]`, "needs a Command"},
		{"long command", `[{"Name": "a", "Command": "` + strings.Repeat("a", 4097) + `"}]`, "at most 4096"},
		{"nul in command", `[{"Name": "a", "Command": "true\u0000"}]`, "NUL"},
		{"negative timeout", `[{"Name": "a", "Command": "true", "TimeoutSeconds": -1}]`, "TimeoutSeconds"},
		{"long timeout", `[{"Name": "a", "Command": "true", "TimeoutSeconds": 86401}]`, "TimeoutSeconds"},
		{"same name twice", `[{"Name": "a", "Command": "true"}, {"Name": "a", "Command": "false"}]`, `already a task named "a"`},
		{"not a list", `{"Name": "a", "Command": "true"}`, "cannot unmarshal"},
	}
	for _, c := range cases {
		if _, err := Parse([]byte(c.file)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: expected an error with %q, got %v", c.name, c.want, err)
		}
	}

	many := "[" + strings.Repeat(`{"Name": "a", "Command": "true"},`, 50) + `{"Name": "b", "Command": "true"}]`
	if _, err := Parse([]byte(many)); err == nil || !strings.Contains(err.Error(), "at most 50") {
		t.Errorf("expected too many tasks to be refused, got %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if tasks, err := Load(filepath.Join(dir, "none.json")); err != nil || tasks != nil {
		t.Errorf("expected no tasks and no error without a file, got %v %v", tasks, err)
	}
	if tasks, err := Load(""); err != nil || tasks != nil {
		t.Errorf("expected no tasks without a path, got %v %v", tasks, err)
	}

	path := filepath.Join(dir, "tasks.json")
	if err := os.WriteFile(path, []byte(`[{"Name": "a", "Command": "true"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if tasks, err := Load(path); err != nil || len(tasks) != 1 {
		t.Errorf("expected one task, got %v %v", tasks, err)
	}

	// whoever can write the file runs commands as the agent
	for _, mode := range []os.FileMode{0620, 0602, 0666} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "chmod go-w") {
			t.Errorf("mode %o: expected the file to be refused, got %v", mode, err)
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Errorf("expected a file others can only read to load, got %v", err)
	}

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("expected a directory to be refused, got %v", err)
	}

	if err := os.WriteFile(path, []byte(`[{"Name": "a"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("expected the path in the error, got %v", err)
	}
}

// the sample that ships with the agent has to be a valid tasks file
func TestSampleFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "agent", "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if tasks, err := Parse(data); err != nil || len(tasks) == 0 {
		t.Errorf("expected agent/tasks.json to parse, got %d tasks, %v", len(tasks), err)
	}
}
