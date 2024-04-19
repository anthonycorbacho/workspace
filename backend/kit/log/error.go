package log

import (
	"fmt"
	"reflect"
	"sync"

	"go.uber.org/zap/zapcore"
)

func encodeError(key string, err error, enc zapcore.ObjectEncoder) (retErr error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			if v := reflect.ValueOf(err); v.Kind() == reflect.Ptr && v.IsNil() {
				enc.AddString(key, "<nil>")
				return
			}

			retErr = fmt.Errorf("PANIC=%v", rerr)
		}
	}()

	basic := err.Error()
	enc.AddString(key, basic)

	switch e := err.(type) {
	case errorGroup:
		return enc.AddArray(key+"Causes", errArray(e.Errors()))
	case fmt.Formatter:
		verbose := fmt.Sprintf("%+v", e)
		if verbose != basic {
			enc.AddString(key+"Verbose", verbose)
		}
	}
	return nil
}

type errorGroup interface {
	Errors() []error
}

type errArray []error

func (errs errArray) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	for i := range errs {
		if errs[i] == nil {
			continue
		}

		el := newErrArrayElem(errs[i])
		_ = arr.AppendObject(el) //nolint
		el.Free()
	}
	return nil
}

var _errArrayElemPool = sync.Pool{New: func() interface{} {
	return &errArrayElem{}
}}

type errArrayElem struct{ err error }

func newErrArrayElem(err error) *errArrayElem {
	e := _errArrayElemPool.Get().(*errArrayElem)
	e.err = err
	return e
}

func (e *errArrayElem) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	return arr.AppendObject(e)
}

func (e *errArrayElem) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return encodeError("error", e.err, enc)
}

func (e *errArrayElem) Free() {
	e.err = nil
	_errArrayElemPool.Put(e)
}
