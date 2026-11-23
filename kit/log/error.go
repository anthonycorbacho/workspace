package log

import (
	"fmt"
	"log/slog"
	"reflect"
)

type errorGroup interface {
	Errors() []error
}

type multiUnwrap interface {
	Unwrap() []error
}

// errorAttrs encodes an error into one or more slog attributes, safely handling
// typed-nil pointers, panicking Error() implementations, error groups, and fmt.Formatter verbose details.
func errorAttrs(key string, err error) []slog.Attr {
	basic, panicked := safeErrorString(err)
	if panicked {
		return []slog.Attr{slog.String(key, basic)}
	}

	attrs := []slog.Attr{slog.String(key, basic)}
	switch e := err.(type) {
	case errorGroup:
		attrs = append(attrs, slog.Any(key+"Causes", encodeErrorSlice(e.Errors())))
	case multiUnwrap:
		attrs = append(attrs, slog.Any(key+"Causes", encodeErrorSlice(e.Unwrap())))
	case fmt.Formatter:
		verbose := fmt.Sprintf("%+v", e)
		if verbose != basic {
			attrs = append(attrs, slog.String(key+"Verbose", verbose))
		}
	}
	return attrs
}

func encodeErrorSlice(errs []error) []map[string]any {
	causes := make([]map[string]any, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		item := make(map[string]any)
		for _, a := range errorAttrs("error", err) {
			item[a.Key] = a.Value.Any()
		}
		causes = append(causes, item)
	}
	return causes
}

func safeErrorString(err error) (msg string, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			if v := reflect.ValueOf(err); v.Kind() == reflect.Pointer && v.IsNil() {
				msg = "<nil>"
				panicked = true
				return
			}
			msg = fmt.Sprintf("PANIC=%v", r)
			panicked = true
		}
	}()
	return err.Error(), false
}
