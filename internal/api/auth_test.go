package api

import (
	"context"
	"testing"

	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeLookup knows one agent key, which belongs to web1
type fakeLookup struct{}

func (fakeLookup) HostForCredential(ctx context.Context, secret string) (string, error) {
	if secret == "web1-key" {
		return "web1", nil
	}
	return "", store.ErrNotFound
}

// call runs the interceptor for a method with the given metadata and
// returns the host the handler saw, if it ran
func call(t *testing.T, method string, meta map[string]string) (string, bool, error) {
	t.Helper()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(meta))
	var host string
	var ran bool
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		ran = true
		host, _ = agentHost(ctx)
		return nil, nil
	}
	_, err := AuthInterceptor(fakeLookup{})(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)
	return host, ran, err
}

func TestEnrollNeedsNoCredential(t *testing.T) {
	if _, ran, err := call(t, MonitorDataService_Enroll_FullMethodName, nil); !ran || err != nil {
		t.Errorf("expected enroll to reach the handler, got %v", err)
	}
}

func TestAgentKeyIdentifiesTheHost(t *testing.T) {
	host, ran, err := call(t, MonitorDataService_HandleMonitorData_FullMethodName, map[string]string{transport.AgentKeyHeader: "web1-key"})
	if !ran || err != nil || host != "web1" {
		t.Errorf("expected the handler to run as web1, got %q %v", host, err)
	}
}

func TestAgentKeyCannotRead(t *testing.T) {
	_, ran, err := call(t, MonitorDataService_Fleet_FullMethodName, map[string]string{transport.AgentKeyHeader: "web1-key"})
	if ran || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected an agent reading the fleet to be denied, got %v", err)
	}
}

func TestUnknownAgentKey(t *testing.T) {
	_, ran, err := call(t, MonitorDataService_HandlePing_FullMethodName, map[string]string{transport.AgentKeyHeader: "stolen"})
	if ran || status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected an unknown key to be rejected, got %v", err)
	}
}

func TestNoCredential(t *testing.T) {
	_, ran, err := call(t, MonitorDataService_HandleMonitorData_FullMethodName, nil)
	if ran || status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected a call without credentials to be rejected, got %v", err)
	}
}

func TestSharedKeyStillWorks(t *testing.T) {
	t.Setenv("SYMON_KEY", "test-key")
	token, err := auth.GenerateJWT()
	if err != nil {
		t.Fatal(err)
	}
	host, ran, err := call(t, MonitorDataService_Fleet_FullMethodName, map[string]string{"jwt": token})
	if !ran || err != nil || host != "" {
		t.Errorf("expected the shared key to pass without an agent host, got %q %v", host, err)
	}
}
