package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
)

func TestEnrollToken(t *testing.T) {
	s, fake := newTestServer(t, nil)
	behindProxy := func(r *http.Request) {
		asJSON(r)
		r.Host = "symon.example.com"
		r.Header.Set("X-Forwarded-Proto", "https")
	}

	rec := call(s, "POST", "/api/v1/enroll-tokens", `{"host":"web-3","uses":1,"hours":24}`, testSession, behindProxy)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("a token should not be cached, got %q", rec.Header().Get("Cache-Control"))
	}
	var reply struct {
		Token   string `json:"token"`
		Expires int64  `json:"expires"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	want := "curl -fsSL --connect-timeout 10 https://symon.example.com/install.sh | sudo sh -s -- --token tok123 --host web-3"
	if reply.Token != "tok123" || reply.Expires != 1700003600 || reply.Command != want {
		t.Errorf("unexpected reply %+v", reply)
	}
	request := fake.lastEnrollToken.Load().(*api.EnrollTokenRequest)
	if request.Host != "web-3" || request.Uses != 1 || request.TtlSeconds != 24*3600 || request.By != "tester" {
		t.Errorf("unexpected request %+v", request)
	}

	// without a host name the script names the host after the machine
	rec = call(s, "POST", "/api/v1/enroll-tokens", `{"uses":5,"hours":1}`, testSession, asJSON)
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || reply.Command != "curl -fsSL --connect-timeout 10 http://example.com/install.sh | sudo sh -s -- --token tok123" {
		t.Errorf("unexpected reply %d %s", rec.Code, rec.Body)
	}

	tests := []struct {
		body    string
		cookie  string
		prepare func(*http.Request)
		code    int
	}{
		{`{"uses":1,"hours":1}`, viewerSession, asJSON, http.StatusForbidden},
		{`{"uses":1,"hours":1}`, "", asJSON, http.StatusUnauthorized},
		{`{"uses":1,"hours":1}`, testSession, nil, http.StatusUnsupportedMediaType},
		{`{"uses":0,"hours":1}`, testSession, asJSON, http.StatusBadRequest},
		{`not json`, testSession, asJSON, http.StatusBadRequest},
		{`{"uses":1,"hours":1}`, testSession, func(r *http.Request) { asJSON(r); r.Host = "a b" }, http.StatusBadRequest},
	}
	for _, tt := range tests {
		if rec := call(s, "POST", "/api/v1/enroll-tokens", tt.body, tt.cookie, tt.prepare); rec.Code != tt.code {
			t.Errorf("%s as %q: got %d %s, want %d", tt.body, tt.cookie, rec.Code, rec.Body, tt.code)
		}
	}
}
