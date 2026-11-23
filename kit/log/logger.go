package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/anthonycorbacho/workspace/kit/config"
	"go.opentelemetry.io/otel/trace"
)

// A Logger provides fast, leveled, structured logging.
// All methods are safe for concurrent use.
type Logger struct {
	handler slog.Handler
	level   Level
	output  io.Writer
	onFatal func()
}

// New is a reasonable production logging configuration.
// Logging is enabled at InfoLevel and above by default (FOUNDATION_LOG_LEVEL, WithLevel).
//
// It uses a JSON encoder, writes to standard error (WithOutput), and enables sampling.
// Stacktraces are automatically included on logs of ErrorLevel and above.
func New(opts ...Option) (*Logger, error) {
	envLevel := config.LookupEnv("FOUNDATION_LOG_LEVEL", "")
	if len(envLevel) == 0 {
		envLevel = "INFO"
	}
	level, err := parse(envLevel)
	if err != nil {
		return nil, err
	}

	o := defaultOptions(level)
	for _, opt := range opts {
		opt(o)
	}

	jsonHandler := newJSONHandler(o.output, o.level)
	sampled := newSamplingHandler(jsonHandler, time.Second, 100, 100)

	return &Logger{
		handler: sampled,
		level:   o.level,
		output:  o.output,
		onFatal: func() { os.Exit(1) },
	}, nil
}

// NewNop returns a no-op Logger. It never writes out logs or internal errors,
// and it never runs user-defined hooks.
func NewNop() *Logger {
	return &Logger{
		handler: slog.DiscardHandler,
		level:   FatalLevel + 1,
	}
}

// Close flushes any buffered log entries if the underlying output supports syncing.
// Applications should take care to call Close before exiting.
func (l *Logger) Close() {
	if l == nil || l.output == nil {
		return
	}
	if syncer, ok := l.output.(interface{ Sync() error }); ok {
		_ = syncer.Sync() //nolint:errcheck // best-effort flush on exit.
	}
}

// Debug logs a message at DebugLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Debug(ctx context.Context, message string, fields ...Field) {
	l.write(DebugLevel, ctx, message, fields...)
}

// Info logs a message at InfoLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Info(ctx context.Context, message string, fields ...Field) {
	l.write(InfoLevel, ctx, message, fields...)
}

// Warn logs a message at WarnLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Warn(ctx context.Context, message string, fields ...Field) {
	l.write(WarnLevel, ctx, message, fields...)
}

// Error logs a message at ErrorLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Error(ctx context.Context, message string, fields ...Field) {
	l.write(ErrorLevel, ctx, message, fields...)
}

// Fatal logs a message at FatalLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
//
// The logger then calls os.Exit(1), even if logging at FatalLevel is
// disabled.
func (l *Logger) Fatal(ctx context.Context, message string, fields ...Field) {
	l.write(FatalLevel, ctx, message, fields...)
}

func (l *Logger) write(level Level, ctx context.Context, msg string, fields ...Field) {
	if l == nil || l.handler == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	slevel := level.slogLevel()
	enabled := l.handler.Enabled(ctx, slevel)
	if !enabled && level < FatalLevel {
		return
	}

	if enabled {
		var pcs [1]uintptr
		// Skip: runtime.Callers -> Logger.write -> Logger.<Level>
		runtime.Callers(3, pcs[:])

		rec := slog.NewRecord(time.Now(), slevel, msg, pcs[0])
		if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
			rec.AddAttrs(
				slog.String("TraceId", sc.TraceID().String()),
				slog.String("SpanId", sc.SpanID().String()),
				slog.String("TraceFlags", sc.TraceFlags().String()),
			)
		}

		rec.AddAttrs(buildAttributes(callerFullPath(pcs[0]), fields))
		if level >= ErrorLevel {
			if stack := captureStacktrace(3); stack != "" {
				rec.AddAttrs(slog.String("Stacktrace", stack))
			}
		}

		if err := l.handler.Handle(ctx, rec); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "log handler error: %v\n", err) //nolint:errcheck // best-effort fallback to stderr.
		}
	}

	if level >= FatalLevel && l.onFatal != nil {
		l.onFatal()
	}
}

func newJSONHandler(w io.Writer, level Level) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level.slogLevel(),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) != 0 {
				return a
			}
			switch a.Key {
			case slog.TimeKey:
				return slog.Int64("Timestamp", a.Value.Time().UnixNano())
			case slog.LevelKey:
				if lvl, ok := a.Value.Any().(slog.Level); ok {
					return slog.String("Severity", formatLevel(lvl))
				}
				return slog.String("Severity", a.Value.String())
			case slog.MessageKey:
				return slog.String("Body", a.Value.String())
			default:
				return a
			}
		},
	})
}

func callerFullPath(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if frame.File == "" {
		return ""
	}
	return frame.File + ":" + strconv.Itoa(frame.Line)
}

func captureStacktrace(skip int) string {
	const maxDepth = 64
	var pcs [maxDepth]uintptr
	n := runtime.Callers(skip+1, pcs[:])
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	for {
		frame, more := frames.Next()
		if frame.Function != "" {
			b.WriteString(frame.Function)
			b.WriteByte('\n')
		}
		b.WriteByte('\t')
		b.WriteString(frame.File)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(frame.Line))
		if !more {
			break
		}
		b.WriteByte('\n')
	}
	return b.String()
}
