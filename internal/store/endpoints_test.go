package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEndpointChecks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	if _, err := st.LatestEndpointCheck(ctx, "api"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound before the first check, got %v", err)
	}

	// api: a check every 2 minutes for an hour, down for the last 3
	start := time.Now().Add(-time.Hour).Truncate(time.Minute)
	for i := 0; i < 30; i++ {
		check := EndpointCheck{Time: start.Add(time.Duration(i) * 2 * time.Minute), Name: "api", URL: "https://api.example.com/health", Method: "GET",
			StatusCode: 200, OK: true, Latency: 100 * time.Millisecond}
		switch {
		case i >= 28:
			check.StatusCode, check.OK, check.Latency, check.Error = 0, false, 0, "connection refused"
		case i == 27:
			check.StatusCode, check.OK, check.Latency = 503, false, 400*time.Millisecond
		}
		if err := st.SaveEndpointCheck(ctx, check); err != nil {
			t.Fatal(err)
		}
	}
	end := start.Add(time.Hour)

	latest, err := st.LatestEndpointCheck(ctx, "api")
	if err != nil || latest.StatusCode != 0 || latest.OK || latest.Error != "connection refused" || !latest.Time.Equal(start.Add(58*time.Minute)) {
		t.Errorf("unexpected latest check %+v %v", latest, err)
	}

	summaries, err := st.Endpoints(ctx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one endpoint, got %+v", summaries)
	}
	summary := summaries[0]
	// 27 of 30 OK, and 28 responses averaging (27*100 + 400) / 28 ms
	if summary.Checks != 30 || summary.UptimePct != 90 || summary.Latest.URL != "https://api.example.com/health" {
		t.Errorf("unexpected summary %+v", summary)
	}
	wantMs := (27.0*100 + 400) / 28
	if want := time.Duration(wantMs * float64(time.Millisecond)); summary.AvgLatency.Round(time.Microsecond) != want.Round(time.Microsecond) {
		t.Errorf("expected an average of %v, got %v", want, summary.AvgLatency)
	}
	// the status at the end of an earlier range is the check before it
	earlier, err := st.Endpoints(ctx, start, start.Add(30*time.Minute))
	if err != nil || len(earlier) != 1 || !earlier[0].Latest.OK || earlier[0].Checks != 15 {
		t.Errorf("unexpected earlier summary %+v %v", earlier, err)
	}

	// buckets of a few seconds, so each check is its own point
	availability, err := st.EndpointSeries(ctx, "api", "availability", start, end, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(availability.Series) != 1 || len(availability.Series[0].Points) != 30 {
		t.Fatalf("expected 30 points, got %+v", availability)
	}
	if points := availability.Series[0].Points; points[26].Value != 100 || points[27].Value != 0 || points[29].Value != 0 {
		t.Errorf("expected the last 3 checks to fail, got %+v", points[26:])
	}
	// the checks without a response have no latency
	latency, err := st.EndpointSeries(ctx, "api", "latency", start, end, 0)
	if err != nil || len(latency.Series[0].Points) != 28 || latency.Series[0].Points[0].Value != 100 || latency.Series[0].Points[27].Value != 400 {
		t.Errorf("expected 28 latencies ending in 400ms, got %+v %v", latency, err)
	}
	if _, err := st.EndpointSeries(ctx, "api", "nope", start, end, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for an unknown metric, got %v", err)
	}
}

func TestAlertWithoutHost(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Truncate(time.Second)
	id, err := st.CreateAlert(ctx, Alert{Rule: "API up", Metric: "endpoint", Target: "https://api.example.com", Severity: 2, StartedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	// a second open alert for the same endpoint rule is refused, like for hosts
	if _, err := st.CreateAlert(ctx, Alert{Rule: "API up", Metric: "endpoint", Target: "https://api.example.com", Severity: 2, StartedAt: at}); err == nil {
		t.Error("expected a second open alert to be refused")
	}
	if _, err := st.CreateAlert(ctx, Alert{Host: "web1", Rule: "API up", Metric: "endpoint", Target: "https://api.example.com", Severity: 2, StartedAt: at}); err != nil {
		t.Errorf("expected a host's alert with the same rule to be separate, got %v", err)
	}

	open, err := st.OpenAlert(ctx, "", "API up", "endpoint", "https://api.example.com")
	if err != nil || open == nil || open.ID != id || open.Host != "" {
		t.Fatalf("expected the open endpoint alert, got %+v %v", open, err)
	}
	alerts, err := st.Alerts(ctx, AlertFilter{OpenOnly: true})
	if err != nil || len(alerts) != 2 {
		t.Errorf("expected both alerts in the list, got %+v %v", alerts, err)
	}
	if err := st.ResolveAlert(ctx, id, 200, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if open, err := st.OpenAlert(ctx, "", "API up", "endpoint", "https://api.example.com"); err != nil || open != nil {
		t.Errorf("expected no open endpoint alert after resolving, got %+v %v", open, err)
	}
}
