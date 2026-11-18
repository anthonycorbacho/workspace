package kit

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// gatewayTLSConfigs creates a private CA and separate server and gateway certificates.
func gatewayTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "gateway-test-ca"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(ca)

	issue := func(serial int64, name string, usage x509.ExtKeyUsage) tls.Certificate {
		public, key, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		cert := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: name},
			DNSNames:     []string{name},
			NotBefore:    template.NotBefore,
			NotAfter:     template.NotAfter,
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, cert, ca, public, private)
		require.NoError(t, err)
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	}
	return &tls.Config{
		Certificates: []tls.Certificate{issue(2, "gateway.test", x509.ExtKeyUsageServerAuth)},
		ClientCAs:    roots,
		MinVersion:   tls.VersionTLS12,
	}, &tls.Config{
		Certificates: []tls.Certificate{issue(3, "gateway-client", x509.ExtKeyUsageClientAuth)},
		RootCAs:      roots,
		ServerName:   "gateway.test",
		MinVersion:   tls.VersionTLS12,
	}
}

type gatewayTLSServer struct {
	pb.UnimplementedSampleAppServiceServer
}

func (gatewayTLSServer) Fetch(ctx context.Context, _ *pb.FetchRequest) (*pb.FetchResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("missing gRPC peer")
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, errors.New("gateway connection must use TLS")
	}
	name := "tls"
	if len(info.State.VerifiedChains) > 0 {
		name = info.State.VerifiedChains[0][0].Subject.CommonName
	}
	return &pb.FetchResponse{Name: name}, nil
}

func TestGatewayTLS(t *testing.T) {
	serverConfig, gatewayConfig := gatewayTLSConfigs(t)
	for _, tt := range []struct {
		name                string
		clientAuth          tls.ClientAuthType
		untrustedServer     bool
		noClientCertificate bool
		wantStatus          int
		wantName            string
	}{
		{name: "TLS", wantStatus: http.StatusOK, wantName: "tls"},
		{
			name: "mutual TLS", clientAuth: tls.RequireAndVerifyClientCert,
			wantStatus: http.StatusOK, wantName: "gateway-client",
		},
		{name: "untrusted server", untrustedServer: true, wantStatus: http.StatusServiceUnavailable},
		{
			name: "client certificate required", clientAuth: tls.RequireAndVerifyClientCert,
			noClientCertificate: true, wantStatus: http.StatusServiceUnavailable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			serverTLS, gatewayTLS := serverConfig.Clone(), gatewayConfig.Clone()
			serverTLS.ClientAuth = tt.clientAuth
			if tt.untrustedServer {
				gatewayTLS.RootCAs = x509.NewCertPool()
			}
			if tt.noClientCertificate {
				gatewayTLS.Certificates = nil
			}
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = lis.Close() })
			f, err := NewFoundation("gateway-tls",
				WithGRPCAddr(lis.Addr().String()),
				WithGRPCServerOptions(grpc.Creds(credentials.NewTLS(serverTLS))),
				WithGatewayDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(gatewayTLS))),
			)
			require.NoError(t, err)
			f.RegisterService(func(srv *grpc.Server) { pb.RegisterSampleAppServiceServer(srv, gatewayTLSServer{}) })
			served := make(chan error, 1)
			go func() { served <- f.grpcServer.Serve(lis) }()
			t.Cleanup(func() {
				f.grpcServer.Stop()
				assert.NoError(t, awaitResult(t, served))
			})
			f.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
				require.NoError(t, pb.RegisterSampleAppServiceHandler(context.Background(), gw, conn))
			})
			require.NoError(t, f.gwErr)
			t.Cleanup(func() { _ = f.gwClient.Close() })
			f.httpRouter.Handle("/", f.gw)

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/users/abcd", nil)
			f.httpServer.Handler.ServeHTTP(rec, req)
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantName != "" {
				assert.JSONEq(t, `{"name":"`+tt.wantName+`"}`, rec.Body.String())
			}
		})
	}
}
