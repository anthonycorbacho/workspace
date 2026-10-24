package blob

import "maps"

// WriteOptions configures how a file is written, see WriteOption.
type WriteOptions struct {
	// UserMetadata is stored alongside the file and returned with it.
	UserMetadata map[string]string
	// UserTags are attached to the file, they can be used by lifecycle or access policies.
	UserTags map[string]string
	// ContentType is the MIME type of the file, implementations use "application/octet-stream" when empty.
	ContentType string

	// PartSize is the size of each part in a multipart upload.
	// When streaming writes this determines the size of the internal buffer,
	// so it is recommended to manually set this value to a reasonable size.
	// Leaving this value at 0 will let the SDK choose a default part size.
	PartSize uint64
}

// WriteOption configures a write.
type WriteOption func(*WriteOptions)

// WithContentTypeCSV sets the content type to "text/csv".
func WithContentTypeCSV() WriteOption {
	return WithContentType("text/csv")
}

// WithContentType sets the MIME type of the file.
func WithContentType(opt string) WriteOption {
	return func(o *WriteOptions) {
		o.ContentType = opt
	}
}

// WithPartSize sets the size of each part in a multipart upload, see WriteOptions.PartSize.
func WithPartSize(opt uint64) WriteOption {
	return func(o *WriteOptions) {
		o.PartSize = opt
	}
}

// WithUserMetadata adds metadata to the file, it can be used multiple times.
func WithUserMetadata(opt map[string]string) WriteOption {
	return func(o *WriteOptions) {
		if o.UserMetadata == nil {
			o.UserMetadata = make(map[string]string)
		}
		maps.Copy(o.UserMetadata, opt)
	}
}

// WithUserTags adds tags to the file, it can be used multiple times.
func WithUserTags(opt map[string]string) WriteOption {
	return func(o *WriteOptions) {
		if o.UserTags == nil {
			o.UserTags = make(map[string]string)
		}
		maps.Copy(o.UserTags, opt)
	}
}
