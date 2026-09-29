package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
)

// endpointTimeout is how long a check waits for the response headers
const endpointTimeout = 30 * time.Second

// runEndpointChecks checks every enabled endpoint rule now and then every
// interval, and stores the results. The rule loop raises the alerts from them.
func runEndpointChecks(interval time.Duration, st *store.Store) {
	checkAll := func() {
		ctx, cancel := transport.Context()
		rules, err := st.EnabledRules(ctx)
		cancel()
		if err != nil {
			logger.Log("error", "cannot read endpoint rules: "+err.Error())
			return
		}
		var wg sync.WaitGroup
		for i := range rules {
			rule := &rules[i]
			if rule.MetricName != monitor.ENDPOINT {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				check := checkEndpoint(context.Background(), rule, time.Now())
				ctx, cancel := transport.Context()
				defer cancel()
				if err := st.SaveEndpointCheck(ctx, check); err != nil {
					logger.Log("error", "cannot save the check of "+rule.Name+": "+err.Error())
				}
			}()
		}
		wg.Wait()
	}

	checkAll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		checkAll()
	}
}

// expectedCode is the status code an endpoint rule wants, 200 when unset
func expectedCode(rule *alerts.AlertConfig) int {
	if rule.ExpectedHTTPCode == 0 {
		return http.StatusOK
	}
	return rule.ExpectedHTTPCode
}

// checkEndpoint makes one request for an endpoint rule. A request that
// fails has status code 0 and the error.
func checkEndpoint(ctx context.Context, rule *alerts.AlertConfig, now time.Time) store.EndpointCheck {
	method := strings.ToUpper(strings.TrimSpace(rule.Method))
	if method == "" {
		method = http.MethodGet
	}
	check := store.EndpointCheck{Time: now, Name: rule.Name, URL: rule.Endpoint, Method: method}

	client, err := endpointClient(rule)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	// checks are minutes apart, so connections are not worth keeping
	defer client.CloseIdleConnections()

	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(rule.POSTBody)
	}
	request, err := http.NewRequestWithContext(ctx, method, rule.Endpoint, body)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	if method == http.MethodPost && rule.POSTContentType != "" {
		request.Header.Set("Content-Type", rule.POSTContentType)
	}

	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	check.Latency = time.Since(started)
	response.Body.Close()
	if response.TLS != nil && len(response.TLS.PeerCertificates) > 0 {
		check.CertExpires = response.TLS.PeerCertificates[0].NotAfter
	}
	check.StatusCode = response.StatusCode
	check.OK = response.StatusCode == expectedCode(rule)
	return check
}

// endpointClient trusts the rule's CA certificate as well, when it has one
func endpointClient(rule *alerts.AlertConfig) (*http.Client, error) {
	client := &http.Client{Timeout: endpointTimeout}
	if strings.TrimSpace(rule.CustomCACert) == "" {
		return client, nil
	}
	pem, err := os.ReadFile(rule.CustomCACert)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("no certificates in " + rule.CustomCACert)
	}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	return client, nil
}
