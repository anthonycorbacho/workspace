package redis

import (
	"context"
	"reflect"
	"time"

	"github.com/anthonycorbacho/workspace/kit/cache"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// enforce the Cache to implement the cache.Cache interface.
var _ cache.Cache = (*Cache)(nil)

// Cache provides a cache.Cache based on Redis.
type Cache struct {
	client *redis.Client
}

// New creates a new Cache with the given redis configuration, with tracing and metrics enabled.
func New(opt *redis.Options) (*Cache, error) {
	// If there is no options, we should stop and return an error.
	if opt == nil {
		return nil, errors.New("redis option missing")
	}
	// create the client
	rdb := redis.NewClient(opt)

	// Enable tracing instrumentation without raw Redis command arguments,
	// so cached values, tokens, and PII are not exported in trace spans.
	if err := redisotel.InstrumentTracing(rdb, redisotel.WithDBStatement(false)); err != nil {
		_ = rdb.Close() //nolint:errcheck // the instrumentation error is more relevant.
		return nil, errors.Wrap(err, "redis tracing")
	}

	// Enable metrics instrumentation.
	if err := redisotel.InstrumentMetrics(rdb); err != nil {
		_ = rdb.Close() //nolint:errcheck // the instrumentation error is more relevant.
		return nil, errors.Wrap(err, "redis metrics")
	}

	return &Cache{
		client: rdb,
	}, nil
}

// Close closes the connection to redis.
func (c *Cache) Close() error {
	if c.client == nil {
		return errors.New("no redis client")
	}
	// the client is kept, using a closed client returns redis.ErrClosed instead of panicking.
	return c.client.Close()
}

// Get implements cache.Cache.
func (c *Cache) Get(ctx context.Context, key string, value any) error {
	if len(key) == 0 {
		return cache.ErrKeyInvalid
	}

	cmd := c.client.Get(ctx, key)
	b, err := cmd.Bytes()
	if err != nil {
		// If key doesn't exist or the cache expired.
		if errors.Is(err, redis.Nil) {
			return cache.ErrNotFound
		}
		return errors.Wrapf(err, "getting value from cache for key '%s'", key)
	}

	return cache.Unmarshal(b, value)
}

// MultiGet implements cache.Cache, value must be a pointer to a slice.
func (c *Cache) MultiGet(ctx context.Context, keys []string, value any) error {
	if len(keys) == 0 {
		return nil
	}

	// Making sure that we are getting the correct interface
	// we are expecting to get a &[]myType
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("value should be a non-nil pointer")
	}

	valueOf := rv.Elem()
	if valueOf.Kind() != reflect.Slice {
		return errors.New("value should be a pointer of slice")
	}

	results, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return errors.Wrapf(err, "redis MGet error, keys is %+v", keys)
	}

	// If no results found, exit and the slice will remain unchanged.
	if len(results) == 0 {
		return nil
	}

	// type represent the type of the slice
	typ := valueOf.Type().Elem()
	for _, result := range results {
		raw, ok := resultBytes(result)
		if !ok {
			continue
		}

		// creating a new value of the slice type
		object := reflect.New(typ).Interface()
		if err := cache.Unmarshal(raw, object); err != nil {
			continue
		}
		// Adding to the slice the value.
		valueOf.Set(reflect.Append(valueOf, reflect.ValueOf(object).Elem()))
	}

	return nil
}

func resultBytes(result any) ([]byte, bool) {
	switch v := result.(type) {
	case string:
		return []byte(v), true
	case []byte:
		return v, true
	default:
		return nil, false
	}
}

// Set implements cache.Cache.
func (c *Cache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	if len(key) == 0 {
		return cache.ErrKeyInvalid
	}

	if value == nil {
		return cache.ErrValueInvalid
	}

	if expiration < 0 && expiration != redis.KeepTTL {
		return cache.ErrExpirationInvalid
	}

	b, err := cache.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "marshalling value for key '%s'", key)
	}

	if err := c.client.Set(ctx, key, b, expiration).Err(); err != nil {
		return errors.Wrapf(err, "saving value to cache for key '%s'", key)
	}

	return nil
}

// Delete implements cache.Cache.
func (c *Cache) Delete(ctx context.Context, key string) error {
	if len(key) == 0 {
		return cache.ErrKeyInvalid
	}

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return errors.Wrapf(err, "deleting value from cache for key '%s'", key)
	}

	return nil
}
