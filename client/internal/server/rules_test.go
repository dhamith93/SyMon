package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	s, fake := newTestServer(t, nil)

	// viewers can read the rules
	rec := call(s, "GET", "/api/v1/rules", "", viewerSession, nil)
	want := `{"rules":[{"id":1,"enabled":true,"rule":{"Name":"CPU","MetricName":"procUsage"},"updatedAt":1700000000,"updatedBy":"alice"}]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("unexpected rules %d %s", rec.Code, rec.Body)
	}

	tests := []struct {
		method string
		url    string
		body   string
		cookie string
		code   int
		want   string
	}{
		{"POST", "/api/v1/rules", `{"enabled":true,"rule":{"Name":"Memory"}}`, testSession, http.StatusCreated, `{"id":7}`},
		{"POST", "/api/v1/rules", `{"enabled":true,"rule":{"Name":"CPU"}}`, testSession, http.StatusConflict, "already a rule"},
		{"POST", "/api/v1/rules", `{"enabled":true,"rule":{"MetricName":"memory"}}`, testSession, http.StatusBadRequest, "needs a Name"},
		{"POST", "/api/v1/rules", `{"enabled":true}`, testSession, http.StatusBadRequest, "send enabled and rule"},
		{"PUT", "/api/v1/rules/3", `{"enabled":false,"rule":{"Name":"CPU"}}`, testSession, 200, `{"id":3}`},
		{"PUT", "/api/v1/rules/404", `{"enabled":false,"rule":{"Name":"x"}}`, testSession, http.StatusNotFound, "no data found"},
		{"PUT", "/api/v1/rules/nope", `{"enabled":false,"rule":{"Name":"x"}}`, testSession, http.StatusNotFound, "no such rule"},
		{"DELETE", "/api/v1/rules/3", "", testSession, 200, `{"id":3}`},
		// viewers cannot change anything
		{"POST", "/api/v1/rules", `{"enabled":true,"rule":{"Name":"Memory"}}`, viewerSession, http.StatusForbidden, "only admins"},
		{"PUT", "/api/v1/rules/3", `{"enabled":true,"rule":{"Name":"CPU"}}`, viewerSession, http.StatusForbidden, "only admins"},
		{"DELETE", "/api/v1/rules/3", "", viewerSession, http.StatusForbidden, "only admins"},
		{"DELETE", "/api/v1/rules/3", "", "", http.StatusUnauthorized, "log in first"},
	}
	for _, tt := range tests {
		rec := call(s, tt.method, tt.url, tt.body, tt.cookie, asJSON)
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s %s %s as %q: got %d %s, want %d", tt.method, tt.url, tt.body, tt.cookie, rec.Code, rec.Body, tt.code)
		}
	}
	if by := fake.lastRuleBy.Load(); by != "tester" {
		t.Errorf("expected the change to be made by tester, got %v", by)
	}
	if rec := call(s, "POST", "/api/v1/rules", `{"enabled":true,"rule":{"Name":"x"}}`, testSession, nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 for a rule that is not JSON, got %d", rec.Code)
	}
}
