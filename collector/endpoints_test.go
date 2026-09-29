package main

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/monitor"
)

func TestCheckEndpoint(t *testing.T) {
	var gotBody, gotType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/down":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/post":
			body, _ := io.ReadAll(r.Body)
			gotBody, gotType = string(body), r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer server.Close()
	now := time.Unix(1700000000, 0)

	rule := &alerts.AlertConfig{Name: "api", MetricName: monitor.ENDPOINT, Endpoint: server.URL + "/"}
	check := checkEndpoint(context.Background(), rule, now)
	// the expected code is 200 when the rule does not say
	if !check.OK || check.StatusCode != 200 || check.Method != "GET" || check.Latency <= 0 || !check.Time.Equal(now) || check.Error != "" {
		t.Errorf("expected an OK check, got %+v", check)
	}

	rule.Endpoint = server.URL + "/down"
	if check := checkEndpoint(context.Background(), rule, now); check.OK || check.StatusCode != 503 {
		t.Errorf("expected a failed check with 503, got %+v", check)
	}

	post := &alerts.AlertConfig{Name: "post", Endpoint: server.URL + "/post", Method: "post", ExpectedHTTPCode: 201,
		POSTBody: `{"ping":true}`, POSTContentType: "application/json"}
	if check := checkEndpoint(context.Background(), post, now); !check.OK || check.Method != "POST" {
		t.Errorf("expected an OK POST, got %+v", check)
	}
	if gotBody != `{"ping":true}` || gotType != "application/json" {
		t.Errorf("unexpected request body %q and type %q", gotBody, gotType)
	}

	// nothing listens there any more
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	rule.Endpoint = closed.URL
	if check := checkEndpoint(context.Background(), rule, now); check.OK || check.StatusCode != 0 || check.Latency != 0 || check.Error == "" {
		t.Errorf("expected a check without a response, got %+v", check)
	}
}

func TestCheckEndpointWithCustomCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	rule := &alerts.AlertConfig{Name: "tls", Endpoint: server.URL}
	notAfter := server.Certificate().NotAfter

	// the test server's certificate is not trusted by default
	if check := checkEndpoint(context.Background(), rule, time.Now()); check.OK || check.Error == "" {
		t.Errorf("expected an untrusted certificate to fail, got %+v", check)
	}

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caPath, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	rule.CustomCACert = caPath
	if check := checkEndpoint(context.Background(), rule, time.Now()); !check.OK || !check.CertExpires.Equal(notAfter) {
		t.Errorf("expected the custom CA to be trusted and the certificate's expiry kept, got %+v", check)
	}

	rule.CustomCACert = filepath.Join(t.TempDir(), "missing.pem")
	if check := checkEndpoint(context.Background(), rule, time.Now()); check.OK || check.Error == "" {
		t.Errorf("expected a missing CA file to fail the check, got %+v", check)
	}
}
