package server

import (
	_ "embed"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed install.sh
var installScript string

var installTemplate = template.Must(template.New("install.sh").Parse(installScript))

// both values end up inside a shell script, so they are checked first
var (
	safeHost = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]+)?$|^\[[0-9A-Fa-f:.]+\](:[0-9]+)?$`)
	// a build, or the signature agents check it with before updating
	agentFile = regexp.MustCompile(`^agent-linux-(amd64|arm64|arm)(\.sig)?$`)
)

// getInstallScript serves the script that installs and enrolls an agent,
// filled in with how this host reached the dashboard
func (s *server) getInstallScript(w http.ResponseWriter, r *http.Request) {
	dashboard, ok := dashboardBase(r)
	if !ok {
		http.Error(w, "unexpected Host header", http.StatusBadRequest)
		return
	}
	collector := s.agentCollectorEndpoint(r.Host)
	if !safeHost.MatchString(collector) {
		http.Error(w, "the agent collector endpoint is not a host:port", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	installTemplate.Execute(w, map[string]string{
		"Dashboard": dashboard,
		"Collector": collector,
	})
}

// dashboardBase is the dashboard's address the way the browser reached it,
// which new hosts and agent updates use too. ok is false for a Host header
// that could not go into a shell command.
func dashboardBase(r *http.Request) (string, bool) {
	if !safeHost.MatchString(r.Host) {
		return "", false
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host, true
}

// agentCollectorEndpoint is how agents on other hosts reach the collector.
// A collector on localhost is reached through the dashboard's host name, and
// so is a setting without a host, like ":9000" in Docker, where the client
// reaches the collector by a name only the containers know.
func (s *server) agentCollectorEndpoint(dashboardHost string) string {
	endpoint := s.collectorEndpoint
	if s.agentCollector != "" {
		endpoint = s.agentCollector
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint
	}
	local := host == "" || (s.agentCollector == "" && (host == "localhost" || net.ParseIP(host).IsLoopback()))
	if !local {
		return endpoint
	}
	name := dashboardHost
	if withoutPort, _, err := net.SplitHostPort(dashboardHost); err == nil {
		name = withoutPort
	}
	return net.JoinHostPort(name, port)
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
	if strings.HasSuffix(name, ".sig") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	http.ServeFile(w, r, path)
}
