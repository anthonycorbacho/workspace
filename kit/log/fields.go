package log

import (
	"fmt"
	"log/slog"
	"reflect"
	"time"
)

// A Field is a named piece of data added to a log message.
type Field struct {
	attr slog.Attr
	err  error
	skip bool
}

// String field.
func String(key string, val string) Field {
	return Field{attr: slog.String(key, val)}
}

// Strings field.
func Strings(key string, val []string) Field {
	return Field{attr: slog.Any(key, val)}
}

// ByteString field.
func ByteString(key string, val []byte) Field {
	return Field{attr: slog.String(key, string(val))}
}

// Stringer field.
func Stringer(key string, val fmt.Stringer) Field {
	return Field{attr: slog.Any(key, safeStringer{v: val})}
}

// Bool field.
func Bool(key string, val bool) Field {
	return Field{attr: slog.Bool(key, val)}
}

// Int field.
func Int(key string, val int) Field {
	return Field{attr: slog.Int(key, val)}
}

// Int32 field.
func Int32(key string, val int32) Field {
	return Field{attr: slog.Int64(key, int64(val))}
}

// Int64 field.
func Int64(key string, val int64) Field {
	return Field{attr: slog.Int64(key, val)}
}

// Uint field.
func Uint(key string, val uint) Field {
	return Field{attr: slog.Uint64(key, uint64(val))}
}

// Uint32 field.
func Uint32(key string, val uint32) Field {
	return Field{attr: slog.Uint64(key, uint64(val))}
}

// Uint64 field.
func Uint64(key string, val uint64) Field {
	return Field{attr: slog.Uint64(key, val)}
}

// Float32 field.
func Float32(key string, val float32) Field {
	return Field{attr: slog.Float64(key, float64(val))}
}

// Float64 field.
func Float64(key string, val float64) Field {
	return Field{attr: slog.Float64(key, val)}
}

// Error field. A nil error is ignored.
func Error(err error) Field {
	if err == nil {
		return Field{skip: true}
	}
	return Field{attr: slog.Attr{Key: "error"}, err: err}
}

// Duration field.
func Duration(key string, val time.Duration) Field {
	return Field{attr: slog.Duration(key, val)}
}

// Time field.
func Time(key string, val time.Time) Field {
	return Field{attr: slog.Time(key, val)}
}

// Any field.
func Any(key string, val any) Field {
	return Field{attr: slog.Any(key, val)}
}

type safeStringer struct {
	v fmt.Stringer
}

var _ slog.LogValuer = safeStringer{}

func (s safeStringer) LogValue() slog.Value {
	if s.v == nil {
		return slog.StringValue("<nil>")
	}
	return slog.StringValue(encodeStringer(s.v))
}

func encodeStringer(s fmt.Stringer) (out string) {
	defer func() {
		if r := recover(); r != nil {
			if v := reflect.ValueOf(s); v.Kind() == reflect.Pointer && v.IsNil() {
				out = "<nil>"
				return
			}
			out = fmt.Sprintf("PANIC=%v", r)
		}
	}()
	return s.String()
}
