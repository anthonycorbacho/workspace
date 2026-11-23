package log

import (
	"fmt"
	"log/slog"
	"strings"
)

// A Level is a logging priority. Higher levels are more important.
type Level int8

const (
	// DebugLevel logs are typically voluminous, and are usually disabled in
	// production.
	DebugLevel = Level(slog.LevelDebug)
	// InfoLevel is the default logging priority.
	InfoLevel = Level(slog.LevelInfo)
	// WarnLevel logs are more important than Info, but don't need individual
	// human review.
	WarnLevel = Level(slog.LevelWarn)
	// ErrorLevel logs are high-priority. If an application is running smoothly,
	// it shouldn't generate any error-level logs.
	ErrorLevel = Level(slog.LevelError)
	// FatalLevel logs a message, then calls os.Exit(1).
	FatalLevel = Level(slog.LevelError + 4)
)

func (l Level) slogLevel() slog.Level {
	return slog.Level(l)
}

func formatLevel(l slog.Level) string {
	switch l {
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO"
	case slog.LevelWarn:
		return "WARN"
	case slog.LevelError:
		return "ERROR"
	case FatalLevel.slogLevel():
		return "FATAL"
	default:
		return l.String()
	}
}

func parse(in string) (Level, error) {
	in = strings.ToLower(in)
	switch in {
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
