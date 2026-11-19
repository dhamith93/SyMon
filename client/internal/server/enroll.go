package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
)

// postEnrollToken makes a token for the Add host dialog, with the command
// that installs an agent with it. The command uses the dashboard's address
// the way the browser reached it, like the install script does.
func (s *server) postEnrollToken(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the token settings as JSON")
		return
	}
	var body struct {
		Host  string `json:"host"`
		Uses  int32  `json:"uses"`
		Hours int64  `json:"hours"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "send host, uses and hours")
		return
	}
	dashboard, ok := dashboardBase(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "unexpected Host header")
		return
	}
	session, _ := s.sessionOf(r)
	token, err := s.collector.CreateEnrollToken(r.Context(), &api.EnrollTokenRequest{
		Host:       body.Host,
		Uses:       body.Uses,
		TtlSeconds: body.Hours * int64(time.Hour/time.Second),
		By:         session.user,
	})
	if err != nil {
		writeGRPCError(w, "enrollment token", err)
		return
	}

	// the collector only accepts host names of letters, digits, dots,
	// dashes and underscores, so the name is safe in the command
	command := "curl -fsSL --connect-timeout 10 " + dashboard + "/install.sh | sudo sh -s -- --token " + token.Token
	if body.Host != "" {
		command += " --host " + body.Host
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"token": token.Token, "expires": token.Expires, "command": command})
}
