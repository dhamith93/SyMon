package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/version"
)

// agentArchs are the builds the downloads folder has, one per CPU
var agentArchs = []string{"amd64", "arm64", "arm"}

// postAgentUpdate asks agents to update themselves to the build the
// dashboard hands out, which has the dashboard's own version. Agents fetch
// it from the dashboard the way the browser reached it, like the install
// script does, and install it only when it is signed.
func (s *server) postAgentUpdate(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the hosts as JSON")
		return
	}
	var body struct {
		Hosts []string `json:"hosts"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || len(body.Hosts) == 0 {
		writeError(w, http.StatusBadRequest, "send the hosts to update")
		return
	}
	if !safeHost.MatchString(r.Host) {
		writeError(w, http.StatusBadRequest, "unexpected Host header")
		return
	}
	for _, arch := range agentArchs {
		if _, err := os.Stat(filepath.Join(s.downloadsDir, "agent-linux-"+arch+".sig")); errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusConflict, "the agent downloads are not signed, so agents would refuse them. Build them with make pack-client")
			return
		}
	}

	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	session, _ := s.sessionOf(r)
	result, err := s.collector.RequestAgentUpdate(r.Context(), &api.AgentUpdateRequest{
		Hosts:       body.Hosts,
		Version:     version.String(),
		DownloadUrl: scheme + "://" + r.Host,
		By:          session.user,
	})
	if err != nil {
		writeGRPCError(w, "agent update", err)
		return
	}
	writeJSON(w, map[string]any{"requested": result.Requested, "version": version.String()})
}
