package log

import (
	"fmt"
	"strings"

	"go.uber.org/zap/zapcore"
)

// A Level is a logging priority. Higher levels are more important.
type Level int8

const (
	// DebugLevel logs are typically voluminous, and are usually disabled in
	// production.
	DebugLevel = Level(zapcore.DebugLevel)
	// InfoLevel is the default logging priority.
	InfoLevel = Level(zapcore.InfoLevel)
	// WarnLevel logs are more important than Info, but don't need individual
	// human review.
	WarnLevel = Level(zapcore.WarnLevel)
	// ErrorLevel logs are high-priority. If an application is running smoothly,
	// it shouldn't generate any error-level logs.
	ErrorLevel = Level(zapcore.ErrorLevel)
	// FatalLevel logs a message, then calls os.Exit(1).
	FatalLevel = Level(zapcore.FatalLevel)
)

func parse(in string) (Level, error) {
	in = strings.ToLower(in)
	switch string(in) {
	case "debug":
		return DebugLevel, nil
	case "info": // make the zero value useful
		return InfoLevel, nil
	case "warn":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	case "fatal":
		return FatalLevel, nil
	}
	return InfoLevel, fmt.Errorf("failed to parse %s to log level", in)
}
