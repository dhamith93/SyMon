// Package transport holds the gRPC server and client setup shared by all
// SyMon components: TLS, the jwt auth interceptor and long-lived connections.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RequestTimeout is the default deadline for a single RPC.
const RequestTimeout = 10 * time.Second

// AgentKeyHeader is the metadata key an enrolled agent sends its own
// credential in
const AgentKeyHeader = "agent-key"

// NewServer returns a gRPC server that runs auth on every call, usually
// AuthInterceptor. TLS is used when tlsEnabled is set.
func NewServer(tlsEnabled bool, certPath string, keyPath string, auth grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	opts := []grpc.ServerOption{grpc.UnaryInterceptor(auth)}
	if tlsEnabled {
		creds, err := ServerCreds(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("cannot load TLS cert %s, key %s: %w", certPath, keyPath, err)
		}
		opts = append(opts, grpc.Creds(creds))
	}
	return grpc.NewServer(opts...), nil
}

// ServerCreds loads a server certificate and key.
func ServerCreds(certPath string, keyPath string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}), nil
}

// ClientCreds trusts the CA at caPath. An empty caPath means plaintext.
func ClientCreds(caPath string) (credentials.TransportCredentials, error) {
	if len(caPath) == 0 {
		return insecure.NewCredentials(), nil
	}

	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to add server CA cert %s", caPath)
	}
	return credentials.NewTLS(&tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}), nil
}

// Dial creates a connection to endpoint that is meant to be kept for the
// life of the process. It connects lazily and reconnects on its own. auth
// is sent with every call, SharedKey() or AgentKey(key), or nil for none.
func Dial(endpoint string, caPath string, auth credentials.PerRPCCredentials) (*grpc.ClientConn, error) {
	creds, err := ClientCreds(caPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load TLS credentials: %w", err)
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if auth != nil {
		opts = append(opts, grpc.WithPerRPCCredentials(auth))
	}
	return grpc.NewClient(endpoint, opts...)
}

// SharedKey authenticates with a fresh jwt signed with SYMON_KEY
func SharedKey() credentials.PerRPCCredentials {
	return jwtCreds{}
}

// AgentKey authenticates as one enrolled agent
func AgentKey(key string) credentials.PerRPCCredentials {
	return agentKeyCreds{key: key}
}

// Context returns a context with the default request timeout.
func Context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RequestTimeout)
}

// AuthInterceptor rejects calls without a valid jwt in the metadata.
func AuthInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		logger.Log("error", "cannot parse meta")
		return nil, status.Error(codes.Unauthenticated, "INTERNAL_SERVER_ERROR")
	}
	if len(meta["jwt"]) != 1 {
		logger.Log("error", "cannot parse meta - token empty")
		return nil, status.Error(codes.Unauthenticated, "token empty")
	}
	if !auth.ValidToken(meta["jwt"][0]) {
		logger.Log("error", "auth error")
		return nil, status.Error(codes.PermissionDenied, "invalid auth token")
	}
	return handler(ctx, req)
}

// jwtCreds adds a new jwt to each outgoing call.
type jwtCreds struct{}

func (jwtCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	token, err := auth.GenerateJWT()
	if err != nil {
		return nil, fmt.Errorf("cannot generate token: %w", err)
	}
	return map[string]string{"jwt": token}, nil
}

// plaintext is still allowed until TLS becomes the default
func (jwtCreds) RequireTransportSecurity() bool {
	return false
}

// agentKeyCreds sends an enrolled agent's credential with each call
type agentKeyCreds struct {
	key string
}

func (c agentKeyCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{AgentKeyHeader: c.key}, nil
}

func (agentKeyCreds) RequireTransportSecurity() bool {
	return false
}
