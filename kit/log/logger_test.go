package log

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// syncBuffer is a concurrent safe buffer used as log output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// entries returns the decoded json log lines.
func (b *syncBuffer) entries(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		entries = append(entries, e)
	}
	return entries
}

// newTestLogger returns a logger writing json entries in the returned buffer.
func newTestLogger(level Level) (*Logger, *syncBuffer) {
	out := &syncBuffer{}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			LevelKey:    "Severity",
			MessageKey:  "Body",
			EncodeLevel: zapcore.CapitalLevelEncoder,
		}),
		zapcore.AddSync(out),
		zapcore.Level(level),
	)
	return &Logger{log: zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic))}, out
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		opts    []Option
		wantErr bool
	}{
		{name: "default level"},
		{name: "env level", env: "debug"},
		{name: "env level is case insensitive", env: "WARN"},
		{name: "with level option", opts: []Option{WithLevel(ErrorLevel)}},
		{name: "invalid env level", env: "verbose", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FOUNDATION_LOG_LEVEL", tt.env)
			if tt.env == "" {
				require.NoError(t, os.Unsetenv("FOUNDATION_LOG_LEVEL"))
			}
			l, err := New(tt.opts...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, l)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, l)
			l.Close()
		})
	}
}

func TestNewLevel(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		opts  []Option
		level zapcore.Level
	}{
		{name: "default is info", level: zapcore.InfoLevel},
		{name: "from env", env: "DEBUG", level: zapcore.DebugLevel},
		{name: "option overrides env", env: "debug", opts: []Option{WithLevel(WarnLevel)}, level: zapcore.WarnLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FOUNDATION_LOG_LEVEL", tt.env)
			if tt.env == "" {
				require.NoError(t, os.Unsetenv("FOUNDATION_LOG_LEVEL"))
			}
			l, err := New(tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.level, l.log.Level())
		})
	}
}

func TestWithLevel(t *testing.T) {
	tests := []struct {
		name  string
		level Level
	}{
		{name: "debug", level: DebugLevel},
		{name: "info", level: InfoLevel},
		{name: "warn", level: WarnLevel},
		{name: "error", level: ErrorLevel},
		{name: "fatal", level: FatalLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{}
			WithLevel(tt.level)(o)
			assert.Equal(t, tt.level, o.level)
		})
	}
}

func TestWithOutput(t *testing.T) {
	out := &syncBuffer{}
	l, err := New(WithOutput(out), WithLevel(InfoLevel))
	require.NoError(t, err)
	l.Info(context.Background(), "hello")
	l.Debug(context.Background(), "filtered")

	entries := out.entries(t)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello", entries[0]["Body"])
	assert.Equal(t, "INFO", entries[0]["Severity"])

	o := defaultOptions(InfoLevel)
	WithOutput(nil)(o)
	assert.Equal(t, os.Stderr, o.output, "a nil output is ignored")
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Level
		wantErr bool
	}{
		{name: "debug", in: "debug", want: DebugLevel},
		{name: "info", in: "info", want: InfoLevel},
		{name: "warn", in: "warn", want: WarnLevel},
		{name: "error", in: "error", want: ErrorLevel},
		{name: "fatal", in: "fatal", want: FatalLevel},
		{name: "upper case", in: "ERROR", want: ErrorLevel},
		{name: "mixed case", in: "WaRn", want: WarnLevel},
		{name: "empty", in: "", want: InfoLevel, wantErr: true},
		{name: "unknown", in: "warning", want: InfoLevel, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoggerLevels(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		level    Level
		log      func(l *Logger)
		severity string
	}{
		{name: "debug", level: DebugLevel, log: func(l *Logger) { l.Debug(ctx, "msg") }, severity: "DEBUG"},
		{name: "info", level: DebugLevel, log: func(l *Logger) { l.Info(ctx, "msg") }, severity: "INFO"},
		{name: "warn", level: DebugLevel, log: func(l *Logger) { l.Warn(ctx, "msg") }, severity: "WARN"},
		{name: "error", level: DebugLevel, log: func(l *Logger) { l.Error(ctx, "msg") }, severity: "ERROR"},
		{name: "debug filtered at info", level: InfoLevel, log: func(l *Logger) { l.Debug(ctx, "msg") }},
		{name: "info filtered at warn", level: WarnLevel, log: func(l *Logger) { l.Info(ctx, "msg") }},
		{name: "warn filtered at error", level: ErrorLevel, log: func(l *Logger) { l.Warn(ctx, "msg") }},
		{name: "error kept at error", level: ErrorLevel, log: func(l *Logger) { l.Error(ctx, "msg") }, severity: "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, out := newTestLogger(tt.level)
			tt.log(l)

			entries := out.entries(t)
			if tt.severity == "" {
				assert.Empty(t, entries)
				return
			}
			require.Len(t, entries, 1)
			assert.Equal(t, tt.severity, entries[0]["Severity"])
			assert.Equal(t, "msg", entries[0]["Body"])
		})
	}
}

func TestLoggerFatal(t *testing.T) {
	l, out := newTestLogger(InfoLevel)
	// the test logger panics instead of calling os.Exit.
	assert.Panics(t, func() { l.Fatal(context.Background(), "fatal msg") })

	entries := out.entries(t)
	require.Len(t, entries, 1)
	assert.Equal(t, "FATAL", entries[0]["Severity"])
	assert.Equal(t, "fatal msg", entries[0]["Body"])
}

func TestLoggerAttributes(t *testing.T) {
	tests := []struct {
		name   string
		fields []Field
		want   map[string]any
	}{
		{name: "no fields", want: map[string]any{}},
		{
			name:   "fields",
			fields: []Field{String("user", "bob"), Int("age", 42), Bool("admin", true)},
			want:   map[string]any{"user": "bob", "age": float64(42), "admin": true},
		},
		{
			name:   "duplicated key keeps the last value",
			fields: []Field{String("user", "bob"), String("user", "alice")},
			want:   map[string]any{"user": "alice"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, out := newTestLogger(InfoLevel)
			l.Info(context.Background(), "msg", tt.fields...)

			entries := out.entries(t)
			require.Len(t, entries, 1)
			attrs, ok := entries[0]["Attributes"].(map[string]any)
			require.True(t, ok, "Attributes must be an object")

			caller, ok := attrs["caller.full_path"].(string)
			require.True(t, ok)
			assert.Contains(t, caller, "logger_test.go")
			delete(attrs, "caller.full_path")
			assert.Equal(t, tt.want, attrs)

			// no trace information without span in context.
			assert.NotContains(t, entries[0], "TraceId")
			assert.NotContains(t, entries[0], "SpanId")
			assert.NotContains(t, entries[0], "TraceFlags")
		})
	}
}

func TestLoggerTrace(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	tests := []struct {
		name      string
		ctx       context.Context
		wantTrace bool
	}{
		{name: "background context", ctx: context.Background()},
		{
			name: "invalid span context",
			ctx:  trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID})),
		},
		{
			name: "valid span context",
			ctx: trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				SpanID:     spanID,
				TraceFlags: trace.FlagsSampled,
			})),
			wantTrace: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, out := newTestLogger(InfoLevel)
			l.Info(tt.ctx, "msg", String("key", "value"))

			entries := out.entries(t)
			require.Len(t, entries, 1)
			assert.Contains(t, entries[0], "Attributes")
			if !tt.wantTrace {
				assert.NotContains(t, entries[0], "TraceId")
				return
			}
			assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", entries[0]["TraceId"])
			assert.Equal(t, "00f067aa0ba902b7", entries[0]["SpanId"])
			assert.Equal(t, "01", entries[0]["TraceFlags"])
		})
	}
}

func TestLoggerConcurrent(t *testing.T) {
	l, out := newTestLogger(InfoLevel)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			l.Info(context.Background(), "msg", Int("i", i))
		})
	}
	wg.Wait()
	assert.Len(t, out.entries(t), 10)
}

func TestNewNop(t *testing.T) {
	l := NewNop()
	require.NotNil(t, l)
	assert.NotPanics(t, func() {
		l.Debug(context.Background(), "msg")
		l.Info(context.Background(), "msg", String("k", "v"))
		l.Warn(context.Background(), "msg")
		l.Error(context.Background(), "msg", Error(assert.AnError))
		l.Close()
	})
}

func TestClose(t *testing.T) {
	tests := []struct {
		name   string
		logger *Logger
	}{
		{name: "nop logger", logger: NewNop()},
		{name: "nil zap logger", logger: &Logger{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, tt.logger.Close)
		})
	}
}

func TestGlobal(t *testing.T) {
	require.NotNil(t, L(), "the default global logger must be usable")

	original := L()
	l, out := newTestLogger(InfoLevel)
	restore := ReplaceGlobal(l)
	assert.Same(t, l, L())

	L().Info(context.Background(), "global msg")
	entries := out.entries(t)
	require.Len(t, entries, 1)
	assert.Equal(t, "global msg", entries[0]["Body"])

	restore()
	assert.Same(t, original, L())
}

func TestGlobalConcurrent(t *testing.T) {
	original := L()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			restore := ReplaceGlobal(NewNop())
			L().Info(context.Background(), "msg")
			restore()
		})
	}
	wg.Wait()
	// restores ran in arbitrary order, put back the original logger.
	ReplaceGlobal(original)
	assert.Same(t, original, L())
}

func TestNewEmptyEnvLevel(t *testing.T) {
	// an empty variable (eg: declared but not set in a manifest) uses the default level.
	t.Setenv("FOUNDATION_LOG_LEVEL", "")
	l, err := New()
	require.NoError(t, err)
	assert.Equal(t, zapcore.InfoLevel, l.log.Level())
}
