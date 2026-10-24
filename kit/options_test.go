package kit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/rs/cors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestNewFoundation(t *testing.T) {
	logger := log.NewNop()

	tests := []struct {
		name    string
		fname   string
		env     map[string]string
		options []Option
		wantErr bool
		want    options
		// number of grpc server options
		wantGrpcOpts int
	}{
		{
			name:    "empty name",
			fname:   "",
			wantErr: true,
		},
		{
			name:  "defaults",
			fname: "test",
			want: options{
				httpAddr:         "0.0.0.0:8080",
				grpcAddr:         "0.0.0.0:8081",
				metricsAddr:      ":9090",
				probesAddr:       ":9091",
				httpWriteTimeout: 15 * time.Second,
				httpReadTimeout:  15 * time.Second,
			},
		},
		{
			name:  "env variables override defaults",
			fname: "test",
			env: map[string]string{
				"FOUNDATION_HTTP_ADDRESS":    "127.0.0.1:1001",
				"FOUNDATION_GRPC_ADDRESS":    "127.0.0.1:1002",
				"FOUNDATION_METRICS_ADDRESS": "127.0.0.1:1003",
				"FOUNDATION_PROBES_ADDRESS":  "127.0.0.1:1004",
			},
			want: options{
				httpAddr:         "127.0.0.1:1001",
				grpcAddr:         "127.0.0.1:1002",
				metricsAddr:      "127.0.0.1:1003",
				probesAddr:       "127.0.0.1:1004",
				httpWriteTimeout: 15 * time.Second,
				httpReadTimeout:  15 * time.Second,
			},
		},
		{
			name:  "options override env variables",
			fname: "test",
			env: map[string]string{
				"FOUNDATION_HTTP_ADDRESS":    "127.0.0.1:1001",
				"FOUNDATION_GRPC_ADDRESS":    "127.0.0.1:1002",
				"FOUNDATION_METRICS_ADDRESS": "127.0.0.1:1003",
				"FOUNDATION_PROBES_ADDRESS":  "127.0.0.1:1004",
			},
			options: []Option{
				WithHTTPAddr("127.0.0.1:2001"),
				WithGRPCAddr("127.0.0.1:2002"),
				WithMetricsAddr("127.0.0.1:2003"),
				WithProbesAddr("127.0.0.1:2004"),
				WithHTTPWriteTimeout(time.Second),
				WithHTTPReadTimeout(2 * time.Second),
			},
			want: options{
				httpAddr:         "127.0.0.1:2001",
				grpcAddr:         "127.0.0.1:2002",
				metricsAddr:      "127.0.0.1:2003",
				probesAddr:       "127.0.0.1:2004",
				httpWriteTimeout: time.Second,
				httpReadTimeout:  2 * time.Second,
			},
		},
		{
			name:  "cors, logger and grpc server options",
			fname: "test",
			options: []Option{
				WithCORS(cors.Options{AllowedOrigins: []string{"https://example.com"}}),
				WithLogger(logger),
				WithGRPCServerOptions(grpc.MaxRecvMsgSize(1024), grpc.MaxSendMsgSize(1024)),
			},
			want: options{
				httpAddr:         "0.0.0.0:8080",
				grpcAddr:         "0.0.0.0:8081",
				metricsAddr:      ":9090",
				probesAddr:       ":9091",
				httpWriteTimeout: 15 * time.Second,
				httpReadTimeout:  15 * time.Second,
				cors:             &cors.Options{AllowedOrigins: []string{"https://example.com"}},
				logger:           logger,
			},
			wantGrpcOpts: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			f, err := NewFoundation(tt.fname, tt.options...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, f)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.fname, f.name)
			assert.Equal(t, tt.want.httpAddr, f.opts.httpAddr)
			assert.Equal(t, tt.want.grpcAddr, f.opts.grpcAddr)
			assert.Equal(t, tt.want.metricsAddr, f.opts.metricsAddr)
			assert.Equal(t, tt.want.probesAddr, f.opts.probesAddr)
			assert.Equal(t, tt.want.httpWriteTimeout, f.opts.httpWriteTimeout)
			assert.Equal(t, tt.want.httpReadTimeout, f.opts.httpReadTimeout)
			assert.Equal(t, tt.want.cors, f.opts.cors)
			assert.Len(t, f.opts.grpcServerOpts, tt.wantGrpcOpts)
			require.NotNil(t, f.logger, "a logger is always set")
			if tt.want.logger != nil {
				assert.Same(t, tt.want.logger, f.logger)
			}
		})
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	tests := []struct {
		name         string
		options      []Option
		wantRead     time.Duration
		wantWrite    time.Duration
		wantHTTPAddr string
	}{
		{
			name:         "defaults",
			wantRead:     15 * time.Second,
			wantWrite:    15 * time.Second,
			wantHTTPAddr: "0.0.0.0:8080",
		},
		{
			name: "custom",
			options: []Option{
				WithHTTPReadTimeout(3 * time.Second),
				WithHTTPWriteTimeout(4 * time.Second),
				WithHTTPAddr("127.0.0.1:1234"),
			},
			wantRead:     3 * time.Second,
			wantWrite:    4 * time.Second,
			wantHTTPAddr: "127.0.0.1:1234",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFoundation("timeouts", tt.options...)
			require.NoError(t, err)
			f.RegisterHTTPHandler("/ping", func(w http.ResponseWriter, _ *http.Request) {})
			require.NotNil(t, f.httpServer)
			assert.Equal(t, tt.wantRead, f.httpServer.ReadTimeout)
			assert.Equal(t, tt.wantWrite, f.httpServer.WriteTimeout)
			assert.Equal(t, tt.wantHTTPAddr, f.httpServer.Addr)
		})
	}
}

func TestCors(t *testing.T) {
	corsOpts := cors.Options{
		AllowedOrigins: []string{"https://allowed.example.com"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost},
		AllowedHeaders: []string{"X-Custom"},
	}

	tests := []struct {
		name        string
		options     []Option
		method      string
		headers     map[string]string
		wantStatus  int
		wantOrigin  string
		wantMethods string
		wantHeaders string
	}{
		{
			name:    "preflight from allowed origin",
			options: []Option{WithCORS(corsOpts)},
			method:  http.MethodOptions,
			headers: map[string]string{
				"Origin":                         "https://allowed.example.com",
				"Access-Control-Request-Method":  http.MethodPost,
				"Access-Control-Request-Headers": "x-custom",
			},
			wantStatus:  http.StatusNoContent,
			wantOrigin:  "https://allowed.example.com",
			wantMethods: http.MethodPost,
			wantHeaders: "x-custom",
		},
		{
			name:    "preflight from disallowed origin",
			options: []Option{WithCORS(corsOpts)},
			method:  http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://evil.example.com",
				"Access-Control-Request-Method": http.MethodPost,
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:    "preflight with disallowed method",
			options: []Option{WithCORS(corsOpts)},
			method:  http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://allowed.example.com",
				"Access-Control-Request-Method": http.MethodDelete,
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:    "simple request from allowed origin",
			options: []Option{WithCORS(corsOpts)},
			method:  http.MethodGet,
			headers: map[string]string{
				"Origin": "https://allowed.example.com",
			},
			wantStatus: http.StatusOK,
			wantOrigin: "https://allowed.example.com",
		},
		{
			name:    "cors disabled",
			options: []Option{},
			method:  http.MethodGet,
			headers: map[string]string{
				"Origin": "https://allowed.example.com",
			},
			wantStatus: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFoundation("cors", tt.options...)
			require.NoError(t, err)
			f.RegisterHTTPHandler("/cors", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}, http.MethodGet, http.MethodPost)

			req := httptest.NewRequest(tt.method, "/cors", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			f.httpServer.Handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
			assert.Equal(t, tt.wantMethods, rec.Header().Get("Access-Control-Allow-Methods"))
			assert.Equal(t, tt.wantHeaders, rec.Header().Get("Access-Control-Allow-Headers"))
		})
	}
}
