package server

import (
	_ "embed"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"text/template"
)

//go:embed install.sh
var installScript string

var installTemplate = template.Must(template.New("install.sh").Parse(installScript))

// both values end up inside a shell script, so they are checked first
var (
	safeHost  = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]+)?$|^\[[0-9A-Fa-f:.]+\](:[0-9]+)?$`)
	agentFile = regexp.MustCompile(`^agent-linux-(amd64|arm64|arm)$`)
)

// getInstallScript serves the script that installs and enrolls an agent,
// filled in with how this host reached the dashboard
func (s *server) getInstallScript(w http.ResponseWriter, r *http.Request) {
	if !safeHost.MatchString(r.Host) {
		http.Error(w, "unexpected Host header", http.StatusBadRequest)
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	collector := s.agentCollectorEndpoint(r.Host)
	if !safeHost.MatchString(collector) {
		http.Error(w, "the agent collector endpoint is not a host:port", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	installTemplate.Execute(w, map[string]string{
		"Dashboard": scheme + "://" + r.Host,
		"Collector": collector,
	})
}

// agentCollectorEndpoint is how agents on other hosts reach the collector.
// A collector on localhost is reached through the dashboard's host name.
func (s *server) agentCollectorEndpoint(dashboardHost string) string {
	if s.agentCollector != "" {
		return s.agentCollector
	}
	host, port, err := net.SplitHostPort(s.collectorEndpoint)
	if err != nil {
		return s.collectorEndpoint
	}
	if host == "" || host == "localhost" || net.ParseIP(host).IsLoopback() {
		name := dashboardHost
		if withoutPort, _, err := net.SplitHostPort(dashboardHost); err == nil {
			name = withoutPort
		}
		return net.JoinHostPort(name, port)
	}
	return s.collectorEndpoint
}

// getDownload serves an agent build from the downloads folder
func (s *server) getDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !agentFile.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.downloadsDir, name)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		http.Error(w, "there is no "+name+" in the downloads folder, build it with make pack-client", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}
