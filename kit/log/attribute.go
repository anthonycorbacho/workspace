package log

import (
	"log/slog"
)

// buildAttributes constructs the Attributes group with caller.full_path and deduplicated fields
// (last value wins for duplicate keys).
func buildAttributes(callerPath string, fields []Field) slog.Attr {
	attrs := make([]slog.Attr, 0, len(fields)+1)
	if callerPath != "" {
		attrs = append(attrs, slog.String("caller.full_path", callerPath))
	}

	for _, f := range fields {
		if f.skip {
			continue
		}
		if f.err != nil {
			for _, ea := range errorAttrs(f.attr.Key, f.err) {
				attrs = setAttr(attrs, ea)
			}
			continue
		}
		attrs = setAttr(attrs, f.attr)
	}

	return slog.Attr{
		Key:   "Attributes",
		Value: slog.GroupValue(attrs...),
	}
}

func setAttr(attrs []slog.Attr, a slog.Attr) []slog.Attr {
	for i := range attrs {
		if attrs[i].Key == a.Key {
			attrs[i] = a
			return attrs
		}
	}
	return append(attrs, a)
}
