package cache

import (
	"context"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// Cache provides a way define how we cache data.
type Cache interface {
	// Get gets the data from the cache and unmarshall to the given value,
	// if the data cannot be parsed into the value, and error will be returned.
	// If the data or the cache expired, cache.ErrNotFound will be returned.
	Get(ctx context.Context, key string, value any) error

	// MultiGet collects the values of multiple keys and appends them to the slice pointed to by value.
	// Unlike Get, missing or expired keys and values that cannot be parsed are skipped,
	// so the slice may contain fewer items than keys.
	MultiGet(ctx context.Context, keys []string, value any) error

	// Set sets the given data to the cache with a duration TTL.
	// if the data already exist in the cache, it will be replaced by the new value and the new duration.
	// If duration is set to Zero (0), the cache will never expire until removed by calling Delete function
	// or cache is flush.
	Set(ctx context.Context, key string, value any, expiration time.Duration) error

	// Delete deletes data from the cache.
	// if the key doesn't exist, nil error will be return.
	Delete(ctx context.Context, key string) error
}

// Marshal returns the encoded bytes of v.
func Marshal(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}

// Unmarshal decodes the encoded data and stores the result
// in the value pointed to by v
func Unmarshal(data []byte, v any) error {
	return msgpack.Unmarshal(data, v)
}
