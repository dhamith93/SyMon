package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dhamith93/SyMon/internal/config"
)

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	content := `# settings
SYMON_TEST_PLAIN=plain value
export SYMON_TEST_EXPORTED=exported
SYMON_TEST_QUOTED="quoted"
SYMON_TEST_SINGLE='a=b'

SYMON_TEST_ALREADY_SET=from file
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"SYMON_TEST_PLAIN", "SYMON_TEST_EXPORTED", "SYMON_TEST_QUOTED", "SYMON_TEST_SINGLE"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	t.Setenv("SYMON_TEST_ALREADY_SET", "from environment")

	if err := config.LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"SYMON_TEST_PLAIN":       "plain value",
		"SYMON_TEST_EXPORTED":    "exported",
		"SYMON_TEST_QUOTED":      "quoted",
		"SYMON_TEST_SINGLE":      "a=b",
		"SYMON_TEST_ALREADY_SET": "from environment",
	}
	for key, value := range want {
		if got := os.Getenv(key); got != value {
			t.Errorf("%s: got %q, want %q", key, got, value)
		}
	}
}

func TestLoadEnvFileMissingIsFine(t *testing.T) {
	if err := config.LoadEnvFile(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Errorf("expected no error for a missing file, got %v", err)
	}
}

func TestLoadEnvFileBadLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.env")
	if err := os.WriteFile(path, []byte("GOOD=1\nnot a setting\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadEnvFile(path); err == nil {
		t.Error("expected an error for a line without =")
	}
}
