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

// Cache provides a cache based on Redis
type Cache struct {
	client *redis.Client
}

// New create a new Cache with the given redis configuration.
func New(opt *redis.Options) (*Cache, error) {
	// If there is no options, we should stop and return an error.
	if opt == nil {
		return nil, errors.New("redis option missing")
	}
	// create the client
	rdb := redis.NewClient(opt)

	// Enable tracing instrumentation.
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		return nil, errors.Wrap(err, "redis tracing")
	}

	// Enable metrics instrumentation.
	if err := redisotel.InstrumentMetrics(rdb); err != nil {
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

	defer func() {
		c.client = nil
	}()

	return c.client.Close()
}

func (c *Cache) Get(ctx context.Context, key string, value interface{}) error {
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
		return errors.Wrapf(err, "unmarshal value of key '%s'", key)
	}

	return cache.Unmarshal(b, value)
}

func (c *Cache) MultiGet(ctx context.Context, keys []string, value interface{}) error {
	if len(keys) == 0 {
		return nil
	}

	// Making sure that we are getting the correct interface
	// we are expecting to get a &[]myType
	typeOf := reflect.TypeOf(value)
	if typeOf.Kind() != reflect.Ptr {
		return errors.New("value should be a pointer")
	}

	valueOf := reflect.ValueOf(value).Elem()
	if valueOf.Kind() != reflect.Slice {
		return errors.New("value should be a pointer of slice")
	}

	results, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return errors.Wrapf(err, "redis MGet error, keys is %+v", keys)
	}

	// If not results found
	// We exist and the slice will be empty.
	if len(results) == 0 {
		return nil
	}

	// type represent the type of the slice
	typ := reflect.TypeOf(value).Elem().Elem()
	for _, result := range results {
		if result == nil {
			continue
		}

		// creating a new value of the slice type
		object := reflect.New(typ).Interface()
		err = cache.Unmarshal([]byte(result.(string)), object)
		if err != nil {
			continue
		}
		// Adding to the slice the value.
		valueOf.Set(reflect.Append(valueOf, reflect.ValueOf(object).Elem()))
	}

	return nil
}

func (c *Cache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if len(key) == 0 {
		return cache.ErrKeyInvalid
	}

	if value == nil {
		return cache.ErrValueInvalid
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

func (c *Cache) Delete(ctx context.Context, key string) error {
	if len(key) == 0 {
		return cache.ErrKeyInvalid
	}

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return errors.Wrapf(err, "deleting value from cache for key '%s'", key)
	}

	return nil
}
