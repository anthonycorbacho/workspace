package log

import (
	"context"
	"runtime"

	"github.com/anthonycorbacho/workspace/kit/config"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// A Logger provides fast, leveled, structured logging.
// All methods are safe for concurrent use.
type Logger struct {
	log *zap.Logger
}

// New is a reasonable production logging configuration.
// Logging is enabled at InfoLevel and above by default.
//
// It uses a JSON encoder, writes to standard error, and enables sampling.
// Stacktraces are automatically included on logs of ErrorLevel and above.
func New(opts ...func(*Option)) (*Logger, error) {
	level, err := parse(config.LookupEnv("FOUNDATION_LOG_LEVEL", "INFO"))
	if err != nil {
		return nil, err
	}

	options := &Option{Level: level}
	for _, o := range opts {
		o(options)
	}

	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapcore.Level(options.Level)),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding: "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:       "Timestamp",
			LevelKey:      "Severity",
			FunctionKey:   zapcore.OmitKey,
			MessageKey:    "Body",
			StacktraceKey: "Stacktrace",
			LineEnding:    zapcore.DefaultLineEnding,
			EncodeLevel:   zapcore.CapitalLevelEncoder,
			EncodeTime:    zapcore.EpochNanosTimeEncoder,
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	log, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{
		log: log,
	}, nil
}

// NewNop returns a no-op Logger. It never writes out logs or internal errors,
// and it never runs user-defined hooks.
func NewNop() *Logger {
	return &Logger{
		log: zap.NewNop(),
	}
}

// Close is flushing any buffered log entries.
// Applications should take care to call Close before exiting.
func (l *Logger) Close() {
	if l.log == nil {
		return
	}

	_ = l.log.Sync() //nolint
}

// Debug logs a message at DebugLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Debug(ctx context.Context, message string, fields ...Field) {
	log(l.log.Debug, ctx, message, fields...)
}

// Info logs a message at InfoLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Info(ctx context.Context, message string, fields ...Field) {
	log(l.log.Info, ctx, message, fields...)
}

// Warn logs a message at WarnLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Warn(ctx context.Context, message string, fields ...Field) {
	log(l.log.Warn, ctx, message, fields...)
}

// Error logs a message at ErrorLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
func (l *Logger) Error(ctx context.Context, message string, fields ...Field) {
	log(l.log.Error, ctx, message, fields...)
}

// Fatal logs a message at FatalLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
//
// The logger then calls os.Exit(1), even if logging at FatalLevel is
// disabled.
func (l *Logger) Fatal(ctx context.Context, message string, fields ...Field) {
	log(l.log.Fatal, ctx, message, fields...)
}

func log(fn func(msg string, fields ...Field), ctx context.Context, msg string, fields ...Field) { //nolint
	attributes := attributeFields(fields...)
	span := trace.SpanFromContext(ctx)

	// If trace information is not set (non trace context)
	// we will not log traceid.
	if !span.SpanContext().IsValid() {
		fn(
			msg,
			attributeField(attributes),
		)
		return
	}

	fn(
		msg,
		String("TraceId", span.SpanContext().TraceID().String()),
		String("SpanId", span.SpanContext().SpanID().String()),
		String("TraceFlags", span.SpanContext().TraceFlags().String()),
		attributeField(attributes),
	)
}

func attributeFields(fields ...Field) *attributes {
	atts := newAttributes()
	caller := zapcore.NewEntryCaller(runtime.Caller(3))
	atts.Add(zap.String("caller.full_path", caller.FullPath()))
	for _, f := range fields {
		atts.Add(f)
	}
	return atts
}
