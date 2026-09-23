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

func TestAgentCollectorSettings(t *testing.T) {
	t.Setenv("SYMON_DISABLED_COLLECTORS", " Temps, ,tcp")
	t.Setenv("SYMON_CONTAINER_AWARE", "true")

	agent := config.GetAgent()
	if len(agent.DisabledCollectors) != 2 || agent.DisabledCollectors[0] != "temps" || agent.DisabledCollectors[1] != "tcp" {
		t.Errorf("disabled collectors were incorrect, got: %v", agent.DisabledCollectors)
	}
	if !agent.ContainerAware {
		t.Error("expected container aware to be set")
	}
}

func TestAgentCollectorDefaults(t *testing.T) {
	t.Setenv("SYMON_DISABLED_COLLECTORS", "")
	t.Setenv("SYMON_CONTAINER_AWARE", "")

	agent := config.GetAgent()
	if len(agent.DisabledCollectors) != 0 || agent.ContainerAware {
		t.Errorf("expected no disabled collectors and container aware off, got: %+v", agent)
	}
}

func TestAgentDefaults(t *testing.T) {
	t.Setenv("SYMON_SERVER_ID", "")
	t.Setenv("SYMON_AGENT_KEY_PATH", "")

	agent := config.GetAgent()
	hostname, _ := os.Hostname()
	if agent.ServerId != hostname {
		t.Errorf("expected the host name %q by default, got %q", hostname, agent.ServerId)
	}
	if agent.AgentKeyPath != "/etc/symon/agent.key" {
		t.Errorf("unexpected default key path %q", agent.AgentKeyPath)
	}
}
