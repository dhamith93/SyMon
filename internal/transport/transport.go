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

// NewServer returns a gRPC server that checks the jwt on every call.
// TLS is used when tlsEnabled is set.
func NewServer(tlsEnabled bool, certPath string, keyPath string) (*grpc.Server, error) {
	opts := []grpc.ServerOption{grpc.UnaryInterceptor(AuthInterceptor)}
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
// life of the process. It connects lazily and reconnects on its own, and
// every call on it carries a fresh jwt.
func Dial(endpoint string, caPath string) (*grpc.ClientConn, error) {
	creds, err := ClientCreds(caPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load TLS credentials: %w", err)
	}
	return grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(jwtCreds{}),
	)
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
