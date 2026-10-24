// Package redis implements cache.Cache on top of Redis.
//
// Values are encoded with cache.Marshal, and the client is instrumented with OpenTelemetry tracing and metrics.
//
//	c, err := redis.New(&goredis.Options{Addr: "localhost:6379"})
//	if err != nil {
//		return err
//	}
//	defer c.Close()
//	err = c.Set(ctx, "key", value, time.Minute)
package redis
