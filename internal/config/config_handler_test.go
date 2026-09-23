package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dhamith93/SyMon/internal/config"
)

func TestGetServicesToMonitor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	data := `[{"Name": "Apache", "ServiceName": "apache2"}, {"Name": "containerd", "ServiceName": "containerd"}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	got := config.GetServicesToMonitor(path)
	if len(got) != 2 {
		t.Fatalf("service count was incorrect, got: %d, want: 2", len(got))
	}
	if got[0].Name != "Apache" || got[0].ServiceName != "apache2" {
		t.Errorf("first service was incorrect, got: %+v", got[0])
	}
}

func TestGetServicesToMonitorMissingFile(t *testing.T) {
	got := config.GetServicesToMonitor(filepath.Join(t.TempDir(), "missing.json"))
	if len(got) != 0 {
		t.Errorf("expected no services, got: %d", len(got))
	}
}

func TestGetServicesToMonitorBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	got := config.GetServicesToMonitor(path)
	if len(got) != 0 {
		t.Errorf("expected no services, got: %d", len(got))
	}
}
