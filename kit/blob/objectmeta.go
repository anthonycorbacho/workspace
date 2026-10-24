package blob

import "time"

// ObjectMeta is the metadata of a file stored in a bucket, as returned by Blob.Head.
type ObjectMeta struct {
	Bucket       string
	Key          string
	ETag         string
	Size         int64
	ContentType  string
	LastModified time.Time
}
