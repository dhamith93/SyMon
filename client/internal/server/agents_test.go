package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/version"
)

func TestAgentUpdate(t *testing.T) {
	s, fake := newTestServer(t, nil)
	version.Version = "v3.2.0"
	t.Cleanup(func() { version.Version = "" })
	s.downloadsDir = t.TempDir()
	body := `{"hosts":["web1","db1"]}`

	// agents would refuse builds that are not signed
	if rec := call(s, "POST", "/api/v1/agents/update", body, testSession, asJSON); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not signed") {
		t.Errorf("expected 409 without signatures, got %d %s", rec.Code, rec.Body)
	}
	for _, arch := range agentArchs {
		if err := os.WriteFile(filepath.Join(s.downloadsDir, "agent-linux-"+arch+".sig"), []byte("signature\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	rec := call(s, "POST", "/api/v1/agents/update", body, testSession, func(r *http.Request) {
		asJSON(r)
		r.Host = "symon.example.com"
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"requested":2,"version":"v3.2.0"}` {
		t.Fatalf("unexpected response %d %s", rec.Code, rec.Body)
	}
	request := fake.lastAgentUpdate.Load().(*api.AgentUpdateRequest)
	if request.Version != "v3.2.0" || request.DownloadUrl != "https://symon.example.com" || request.By != "tester" || len(request.Hosts) != 2 {
		t.Errorf("unexpected request %+v", request)
	}

	tests := []struct {
		body    string
		cookie  string
		prepare func(*http.Request)
		code    int
	}{
		{body, viewerSession, asJSON, http.StatusForbidden},
		{`{"hosts":[]}`, testSession, asJSON, http.StatusBadRequest},
		{body, testSession, nil, http.StatusUnsupportedMediaType},
		{body, "", asJSON, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		if rec := call(s, "POST", "/api/v1/agents/update", tt.body, tt.cookie, tt.prepare); rec.Code != tt.code {
			t.Errorf("%s as %q: got %d %s, want %d", tt.body, tt.cookie, rec.Code, rec.Body, tt.code)
		}
	}

	// the signatures are downloads too, without a login
	rec = call(s, "GET", "/downloads/agent-linux-arm64.sig", "", "", nil)
	if rec.Code != 200 || rec.Body.String() != "signature\n" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("unexpected signature download %d %q %q", rec.Code, rec.Body, rec.Header().Get("Content-Type"))
	}
}
