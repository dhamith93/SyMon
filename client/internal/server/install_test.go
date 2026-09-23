package server

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func installScriptFor(t *testing.T, s *server, host string) (int, string) {
	t.Helper()
	request := httptest.NewRequest("GET", "/install.sh", nil)
	request.Host = host
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, request)
	return rec.Code, rec.Body.String()
}

func TestInstallScriptUsesTheDashboardHost(t *testing.T) {
	s := &server{collectorEndpoint: "localhost:9000"}
	code, script := installScriptFor(t, s, "symon:8080")
	if code != 200 {
		t.Fatalf("got %d: %s", code, script)
	}
	for _, want := range []string{"DASHBOARD='http://symon:8080'", "COLLECTOR='symon:9000'"} {
		if !strings.Contains(script, want) {
			t.Errorf("expected %q in the script", want)
		}
	}
}

func TestInstallScriptCollectorEndpoint(t *testing.T) {
	tests := []struct {
		collector, override, host, want string
	}{
		{"127.0.0.1:9000", "", "10.0.0.5:8080", "10.0.0.5:9000"},
		{"db.lan:9000", "", "symon:8080", "db.lan:9000"},
		{"localhost:9000", "monitor.example.com:443", "symon:8080", "monitor.example.com:443"},
		{"localhost:9000", "", "[fd00::5]:8080", "[fd00::5]:9000"},
	}
	for _, tt := range tests {
		s := &server{collectorEndpoint: tt.collector, agentCollector: tt.override}
		if got := s.agentCollectorEndpoint(tt.host); got != tt.want {
			t.Errorf("%s via %s: got %s, want %s", tt.collector, tt.host, got, tt.want)
		}
	}
}

func TestInstallScriptRejectsOddHosts(t *testing.T) {
	s := &server{collectorEndpoint: "localhost:9000"}
	for _, host := range []string{"symon';rm -rf /;'", "a b", "$(reboot)"} {
		if code, _ := installScriptFor(t, s, host); code != 400 {
			t.Errorf("%q: expected 400, got %d", host, code)
		}
	}
}

func TestInstallScriptIsValidShell(t *testing.T) {
	s := &server{collectorEndpoint: "localhost:9000"}
	_, script := installScriptFor(t, s, "symon:8080")
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", path).CombinedOutput(); err != nil {
		t.Errorf("sh -n: %v\n%s", err, out)
	}
	if _, err := exec.LookPath("shellcheck"); err == nil {
		if out, err := exec.Command("shellcheck", "-s", "sh", path).CombinedOutput(); err != nil {
			t.Errorf("shellcheck: %v\n%s", err, out)
		}
	}
}

func TestDownloads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent-linux-arm64"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	s := &server{downloadsDir: dir}

	tests := []struct {
		url  string
		code int
		body string
	}{
		{"/downloads/agent-linux-arm64", 200, "binary"},
		{"/downloads/agent-linux-amd64", 404, "make pack-client"},
		{"/downloads/secret.txt", 404, ""},
		{"/downloads/..%2Fsecret.txt", 404, ""},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		s.routes().ServeHTTP(rec, httptest.NewRequest("GET", tt.url, nil))
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.body) {
			t.Errorf("%s: got %d %q, want %d containing %q", tt.url, rec.Code, rec.Body.String(), tt.code, tt.body)
		}
	}
}
