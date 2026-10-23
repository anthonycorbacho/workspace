package blob

import "maps"

type WriteOptions struct {
	UserMetadata map[string]string
	UserTags     map[string]string
	ContentType  string

	// PartSize is the size of each part in a multipart upload.
	// When streaming writes this determines the size of the internal buffer,
	// so it is recommended to manually set this value to a reasonable size.
	// Leaving this value at 0 will let the SDK choose a default part size.
	PartSize uint64
}

type WriteOption func(*WriteOptions)

func WithContentTypeCsv() WriteOption {
	return WithContentType("text/csv")
}

func WithContentType(opt string) WriteOption {
	return func(o *WriteOptions) {
		o.ContentType = opt
	}
}

func WithPartSize(opt uint64) WriteOption {
	return func(o *WriteOptions) {
		o.PartSize = opt
	}
}

func WithUserMetadata(opt map[string]string) WriteOption {
	return func(o *WriteOptions) {
		if o.UserMetadata == nil {
			o.UserMetadata = make(map[string]string)
		}
		maps.Copy(o.UserMetadata, opt)
	}
}

func WithUserTags(opt map[string]string) WriteOption {
	return func(o *WriteOptions) {
		if o.UserTags == nil {
			o.UserTags = make(map[string]string)
		}
		maps.Copy(o.UserTags, opt)
	}
}
