package blob

import "time"

type ObjectMeta struct {
	Bucket       string
	Key          string
	ETag         string
	Size         int64
	ContentType  string
	LastModified time.Time
}
