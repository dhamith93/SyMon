package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAgentUpdates(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, host := range []string{"new1", "old1"} {
		if err := st.AddHost(ctx, host, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()

	// new1 can update itself, old1 is from before updates and says nothing
	if update, err := st.AgentCheckIn(ctx, "new1", now, "v3.1.0", "arm64", ""); err != nil || update != nil {
		t.Fatalf("expected no update yet, got %+v %v", update, err)
	}
	if _, err := st.AgentCheckIn(ctx, "old1", now, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AgentCheckIn(ctx, "ghost", now, "", "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown host, got %v", err)
	}

	requested, err := st.RequestAgentUpdate(ctx, []string{"new1", "old1"}, "v3.2.0", "https://symon.example.com")
	if err != nil || requested != 1 {
		t.Fatalf("expected only new1 to be asked, got %d %v", requested, err)
	}
	// an agent on that version already is not asked again
	if requested, err := st.RequestAgentUpdate(ctx, []string{"new1"}, "v3.1.0", "https://symon.example.com"); err != nil || requested != 0 {
		t.Errorf("expected no request for the version it runs, got %d %v", requested, err)
	}

	update, err := st.AgentCheckIn(ctx, "new1", now, "v3.1.0", "arm64", "")
	if err != nil || update == nil || update.Version != "v3.2.0" || update.URL != "https://symon.example.com" || update.RequestedAt.IsZero() {
		t.Fatalf("expected the update on the next ping, got %+v %v", update, err)
	}

	// a failed try is kept for the dashboard, and the update stays
	if update, err := st.AgentCheckIn(ctx, "new1", now, "v3.1.0", "arm64", "the build is not signed with this agent's update key"); err != nil || update == nil {
		t.Fatalf("expected the update to stay after a failure, got %+v %v", update, err)
	}
	fleet, err := st.FleetSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var new1, old1 HostSummary
	for _, host := range fleet {
		switch host.Name {
		case "new1":
			new1 = host
		case "old1":
			old1 = host
		}
	}
	if !new1.CanUpdate || new1.AgentVersion != "v3.1.0" || new1.UpdateVersion != "v3.2.0" || new1.UpdateError == "" || new1.UpdateRequestedAt.IsZero() {
		t.Errorf("unexpected new1 %+v", new1)
	}
	if old1.CanUpdate || old1.UpdateVersion != "" {
		t.Errorf("unexpected old1 %+v", old1)
	}

	// running the new version clears it
	if update, err := st.AgentCheckIn(ctx, "new1", now, "v3.2.0", "arm64", ""); err != nil || update != nil {
		t.Errorf("expected the update to clear, got %+v %v", update, err)
	}
	fleet, _ = st.FleetSummary(ctx)
	for _, host := range fleet {
		if host.Name == "new1" && (host.UpdateVersion != "" || host.UpdateError != "" || host.AgentVersion != "v3.2.0") {
			t.Errorf("expected new1 up to date, got %+v", host)
		}
	}
}
