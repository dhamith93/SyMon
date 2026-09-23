package transport_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type fakeCollector struct {
	api.UnimplementedMonitorDataServiceServer
}

func (fakeCollector) IsUp(ctx context.Context, in *api.ServerInfo) (*api.IsActive, error) {
	return &api.IsActive{IsUp: true}, nil
}

// startServer runs a collector stub and returns its address
func startServer(t *testing.T, tlsEnabled bool, certPath string, keyPath string) string {
	t.Helper()
	grpcServer, err := transport.NewServer(tlsEnabled, certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	api.RegisterMonitorDataServiceServer(grpcServer, fakeCollector{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)
	return lis.Addr().String()
}

func callIsUp(t *testing.T, addr string, caPath string) error {
	t.Helper()
	conn, err := transport.Dial(addr, caPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := transport.Context()
	defer cancel()
	_, err = api.NewMonitorDataServiceClient(conn).IsUp(ctx, &api.ServerInfo{ServerName: "test"})
	return err
}

func TestDialPlaintext(t *testing.T) {
	t.Setenv("SYMON_KEY", "test-key")
	addr := startServer(t, false, "", "")

	if err := callIsUp(t, addr, ""); err != nil {
		t.Fatalf("expected call to succeed, got: %v", err)
	}
}

func TestMissingTokenRejected(t *testing.T) {
	t.Setenv("SYMON_KEY", "test-key")
	addr := startServer(t, false, "", "")

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := transport.Context()
	defer cancel()
	_, err = api.NewMonitorDataServiceClient(conn).IsUp(ctx, &api.ServerInfo{})

	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %v", err)
	}
}

func TestDialTLS(t *testing.T) {
	t.Setenv("SYMON_KEY", "test-key")
	certPath, keyPath := writeSelfSignedCert(t)
	addr := startServer(t, true, certPath, keyPath)

	if err := callIsUp(t, addr, certPath); err != nil {
		t.Fatalf("expected TLS call to succeed, got: %v", err)
	}
}

func TestClientCredsBadCA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a cert"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.ClientCreds(path); err == nil {
		t.Error("expected an error for a bad CA file")
	}
}

// writeSelfSignedCert writes a cert for 127.0.0.1 that also acts as its own CA
func writeSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "symon-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
