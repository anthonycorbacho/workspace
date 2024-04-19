package log

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// A FieldType indicates which member of the Field union struct should be used
// and how it should be serialized.
type FieldType uint8

// Field types
const (
	UnknownType         = FieldType(zapcore.UnknownType)
	ArrayMarshalerType  = FieldType(zapcore.ArrayMarshalerType)
	ObjectMarshalerType = FieldType(zapcore.ObjectMarshalerType)
	BinaryType          = FieldType(zapcore.BinaryType)
	BoolType            = FieldType(zapcore.BoolType)
	ByteStringType      = FieldType(zapcore.ByteStringType)
	Complex128Type      = FieldType(zapcore.Complex128Type)
	Complex64Type       = FieldType(zapcore.Complex64Type)
	DurationType        = FieldType(zapcore.DurationType)
	Float64Type         = FieldType(zapcore.Float64Type)
	Float32Type         = FieldType(zapcore.Float32Type)
	Int64Type           = FieldType(zapcore.Int64Type)
	Int32Type           = FieldType(zapcore.Int32Type)
	Int16Type           = FieldType(zapcore.Int16Type)
	Int8Type            = FieldType(zapcore.Int8Type)
	StringType          = FieldType(zapcore.StringType)
	TimeType            = FieldType(zapcore.TimeType)
	Uint64Type          = FieldType(zapcore.Uint64Type)
	Uint32Type          = FieldType(zapcore.Uint32Type)
	Uint16Type          = FieldType(zapcore.Uint16Type)
	Uint8Type           = FieldType(zapcore.Uint8Type)
	UintptrType         = FieldType(zapcore.UintptrType)
	ReflectType         = FieldType(zapcore.ReflectType)
	NamespaceType       = FieldType(zapcore.NamespaceType)
	StringerType        = FieldType(zapcore.StringerType)
	ErrorType           = FieldType(zapcore.ErrorType)
	SkipType            = FieldType(zapcore.SkipType)
)

// A Field is a named piece of data added to a log message.
type Field = zap.Field

// String field.
func String(key string, val string) Field {
	return zap.String(key, val)
}

// Strings field
func Strings(key string, val []string) Field {
	return zap.Strings(key, val)
}

// ByteString field.
func ByteString(key string, val []byte) Field {
	return zap.ByteString(key, val)
}

// Stringer field.
func Stringer(key string, val fmt.Stringer) Field {
	return zap.Stringer(key, val)
}

// Bool field.
func Bool(key string, val bool) Field {
	return zap.Bool(key, val)
}

// Int field.
func Int(key string, val int) Field {
	return zap.Int(key, val)
}

// Int32 field.
func Int32(key string, val int32) Field {
	return zap.Int32(key, val)
}

// Int64 field.
func Int64(key string, val int64) Field {
	return zap.Int64(key, val)
}

// Uint field.
func Uint(key string, val uint) Field {
	return zap.Uint(key, val)
}

// Uint32 field.
func Uint32(key string, val uint32) Field {
	return zap.Uint32(key, val)
}

// Uint64 field.
func Uint64(key string, val uint64) Field {
	return zap.Uint64(key, val)
}

// Float32 field.
func Float32(key string, val float32) Field {
	return zap.Float32(key, val)
}

// Float64 field.
func Float64(key string, val float64) Field {
	return zap.Float64(key, val)
}

// Error field.
func Error(err error) Field {
	return zap.Error(err)
}

// Duration field.
func Duration(key string, val time.Duration) Field {
	return zap.Duration(key, val)
}

// Time field.
func Time(key string, val time.Time) Field {
	return zap.Time(key, val)
}

// Any field.
func Any(key string, val interface{}) Field {
	return zap.Any(key, val)
}
