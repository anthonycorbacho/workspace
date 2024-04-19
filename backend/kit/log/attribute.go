package log

import (
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type attributes struct {
	store map[string]zap.Field
	mutex *sync.RWMutex
}

func attributeField(l *attributes) zap.Field {
	return zap.Object("Attributes", l)
}

func newAttributes() *attributes {
	return &attributes{store: map[string]zap.Field{}, mutex: &sync.RWMutex{}}
}

func (a *attributes) Add(field zap.Field) {
	a.mutex.Lock()
	a.store[field.Key] = field
	a.mutex.Unlock()
}

//nolint
func (a attributes) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	var err error
	for k, v := range a.store {
		switch v.Type {
		case zapcore.ArrayMarshalerType:
			err = enc.AddArray(k, v.Interface.(zapcore.ArrayMarshaler))
		case zapcore.ObjectMarshalerType:
			err = enc.AddObject(k, v.Interface.(zapcore.ObjectMarshaler))
		case zapcore.InlineMarshalerType:
			err = v.Interface.(zapcore.ObjectMarshaler).MarshalLogObject(enc)
		case zapcore.BinaryType:
			enc.AddBinary(k, v.Interface.([]byte))
		case zapcore.BoolType:
			enc.AddBool(k, v.Integer == 1)
		case zapcore.ByteStringType:
			enc.AddByteString(k, v.Interface.([]byte))
		case zapcore.Complex128Type:
			enc.AddComplex128(k, v.Interface.(complex128))
		case zapcore.Complex64Type:
			enc.AddComplex64(k, v.Interface.(complex64))
		case zapcore.DurationType:
			enc.AddDuration(k, time.Duration(v.Integer))
		case zapcore.Float64Type:
			enc.AddFloat64(k, math.Float64frombits(uint64(v.Integer)))
		case zapcore.Float32Type:
			enc.AddFloat32(k, math.Float32frombits(uint32(v.Integer)))
		case zapcore.Int64Type:
			enc.AddInt64(k, v.Integer)
		case zapcore.Int32Type:
			enc.AddInt32(k, int32(v.Integer))
		case zapcore.Int16Type:
			enc.AddInt16(k, int16(v.Integer))
		case zapcore.Int8Type:
			enc.AddInt8(k, int8(v.Integer))
		case zapcore.StringType:
			enc.AddString(k, v.String)
		case zapcore.TimeType:
			if v.Interface != nil {
				enc.AddTime(k, time.Unix(0, v.Integer).In(v.Interface.(*time.Location)))
			} else {
				// Fall back to UTC if location is nil.
				enc.AddTime(k, time.Unix(0, v.Integer))
			}
		case zapcore.TimeFullType:
			enc.AddTime(k, v.Interface.(time.Time))
		case zapcore.Uint64Type:
			enc.AddUint64(k, uint64(v.Integer))
		case zapcore.Uint32Type:
			enc.AddUint32(k, uint32(v.Integer))
		case zapcore.Uint16Type:
			enc.AddUint16(k, uint16(v.Integer))
		case zapcore.Uint8Type:
			enc.AddUint8(k, uint8(v.Integer))
		case zapcore.UintptrType:
			enc.AddUintptr(k, uintptr(v.Integer))
		case zapcore.ReflectType:
			err = enc.AddReflected(k, v.Interface)
		case zapcore.NamespaceType:
			enc.OpenNamespace(k)
		case zapcore.StringerType:
			err = encodeStringer(k, v.Interface, enc)
		case zapcore.ErrorType:
			err = encodeError(k, v.Interface.(error), enc)
		case zapcore.SkipType:
			break
		default:
			err = fmt.Errorf("unknown field type: %v", v.Type)
		}
	}

	return err
}

func encodeStringer(key string, stringer interface{}, enc zapcore.ObjectEncoder) (retErr error) {
	defer func() {
		if err := recover(); err != nil {
			if v := reflect.ValueOf(stringer); v.Kind() == reflect.Ptr && v.IsNil() {
				enc.AddString(key, "<nil>")
				return
			}

			retErr = fmt.Errorf("PANIC=%v", err)
		}
	}()

	enc.AddString(key, stringer.(fmt.Stringer).String())
	return nil
}
