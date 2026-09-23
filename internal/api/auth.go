package api

import (
	"context"
	"errors"

	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// agentMethods are the calls an enrolled agent may make with its own key
var agentMethods = map[string]bool{
	MonitorDataService_HandlePing_FullMethodName:              true,
	MonitorDataService_HandleMonitorData_FullMethodName:       true,
	MonitorDataService_HandleCustomMonitorData_FullMethodName: true,
}

type credentialLookup interface {
	HostForCredential(ctx context.Context, secret string) (string, error)
}

// AuthInterceptor checks every call to the collector. Enroll needs no
// credential, its token is the credential. A call with an agent key may
// only send data, and is treated as coming from that agent's host. All
// other calls need the shared key jwt, which older agents also use.
func AuthInterceptor(lookup credentialLookup) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info.FullMethod == MonitorDataService_Enroll_FullMethodName {
			return handler(ctx, req)
		}

		meta, _ := metadata.FromIncomingContext(ctx)
		keys := meta.Get(transport.AgentKeyHeader)
		if len(keys) == 0 {
			return transport.AuthInterceptor(ctx, req, info, handler)
		}

		if len(keys) != 1 || !agentMethods[info.FullMethod] {
			return nil, status.Error(codes.PermissionDenied, "agents may only send data")
		}
		host, err := lookup.HostForCredential(ctx, keys[0])
		if errors.Is(err, store.ErrNotFound) {
			return nil, status.Error(codes.Unauthenticated, "unknown agent key, enroll this host again")
		}
		if err != nil {
			return nil, toStatus(err)
		}
		return handler(context.WithValue(ctx, agentHostKey{}, host), req)
	}
}

type agentHostKey struct{}

// agentHost returns the host an agent key authenticated as. Data from
// such a call is stored under this host, whatever the payload says.
func agentHost(ctx context.Context) (string, bool) {
	host, ok := ctx.Value(agentHostKey{}).(string)
	return host, ok
}
