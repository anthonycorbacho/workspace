package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/cache"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type redisTestSuite struct {
	suite.Suite
	cache *Cache
}

func TestRedisTestSuite(t *testing.T) {
	suite.Run(t, new(redisTestSuite))
}

func (r *redisTestSuite) SetupSuite() {
	if os.Getenv("TESTINGREDIS_URL") == "" {
		r.T().Skip("Skipping, no testing redis setup via env variable TESTINGREDIS_URL")
	}

	addr, _ := os.LookupEnv("TESTINGREDIS_URL")
	c, err := New(&redis.Options{
		Addr: addr,
	})
	if err != nil {
		r.T().Fatalf("setting up redis client %v", err)
	}
	r.cache = c

}

func (r *redisTestSuite) TearDownSuite() {
	r.cache.Close()
}

func (r *redisTestSuite) TestSetAndGet() {
	// Given
	ctx := context.TODO()
	type myStruct struct {
		Value  string
		Number int
		Float  float64
	}

	mystruct := myStruct{
		Value:  "This is a string value",
		Number: 42,
		Float:  42.42,
	}

	// Setting up the case
	key := fmt.Sprintf("key_%s", id.New())
	defer r.cache.Delete(ctx, key)

	err := r.cache.Set(ctx, key, mystruct, 0)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}

	mystruct2 := myStruct{}
	err = r.cache.Get(ctx, key, &mystruct2)
	if err != nil {
		assert.Fail(r.T(), "getting data from cache %v", err)
	}

	assert.Equal(r.T(), mystruct, mystruct2)
}

func (r *redisTestSuite) TestSetAndMultiGet() {
	// Given
	ctx := context.TODO()
	type myStruct struct {
		Value  string
		Number int
		Float  float64
	}

	key := fmt.Sprintf("key_%s", id.New())
	mystruct := myStruct{
		Value:  "This is a string value",
		Number: 42,
		Float:  42.42,
	}

	key2 := fmt.Sprintf("key_%s", id.New())
	mystruct2 := myStruct{
		Value:  "This is a string value 2",
		Number: 422,
		Float:  422.422,
	}

	err := r.cache.Set(ctx, key, mystruct, 0)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}

	err = r.cache.Set(ctx, key2, mystruct2, 0)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}

	// Valid test
	mystructs := []myStruct{}
	err = r.cache.MultiGet(ctx, []string{key, key2}, &mystructs)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}
	r.Equal(mystruct, mystructs[0])
	r.Equal(mystruct2, mystructs[1])

	// Test with one non-existing input
	mystructs = []myStruct{}
	err = r.cache.MultiGet(ctx, []string{key, "invalid"}, &mystructs)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}
	r.Equal(mystruct, mystructs[0])
	r.Nil(err)

	// Test with non existing
	mystructs = []myStruct{}
	err = r.cache.MultiGet(ctx, []string{"not_exist"}, &mystructs)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}
	r.Equal(mystructs, []myStruct{})
	r.Nil(err)
}

func (r *redisTestSuite) TestSetAndDelete() {
	// Given
	ctx := context.TODO()

	// Setting up the case
	key := fmt.Sprintf("key_%s", id.New())
	err := r.cache.Set(ctx, key, 42, 0)
	if err != nil {
		assert.Fail(r.T(), "setting data into cache %v", err)
	}

	err = r.cache.Delete(ctx, key)
	if err != nil {
		assert.Fail(r.T(), "deleting data from cache %v", err)
	}

	var noop int
	err = r.cache.Get(ctx, key, &noop)
	assert.Empty(r.T(), noop)
	assert.Equal(r.T(), err, cache.ErrNotFound)
}

type cacheItem struct {
	Name  string
	Count int
	Tags  []string
}

func (r *redisTestSuite) TestSetAndGetTypes() {
	ctx := context.TODO()
	tests := []struct {
		name  string
		value any
		// get returns a pointer to decode the value into and the decoded value.
		get func() (any, func() any)
	}{
		{
			name:  "struct",
			value: cacheItem{Name: "item", Count: 2, Tags: []string{"a", "b"}},
			get: func() (any, func() any) {
				var v cacheItem
				return &v, func() any { return v }
			},
		},
		{
			name:  "string",
			value: "hello",
			get: func() (any, func() any) {
				var v string
				return &v, func() any { return v }
			},
		},
		{
			name:  "int",
			value: 42,
			get: func() (any, func() any) {
				var v int
				return &v, func() any { return v }
			},
		},
		{
			name:  "bool",
			value: true,
			get: func() (any, func() any) {
				var v bool
				return &v, func() any { return v }
			},
		},
		{
			name:  "slice",
			value: []int{1, 2, 3},
			get: func() (any, func() any) {
				var v []int
				return &v, func() any { return v }
			},
		},
		{
			name:  "map",
			value: map[string]int{"a": 1},
			get: func() (any, func() any) {
				var v map[string]int
				return &v, func() any { return v }
			},
		},
	}
	for _, tt := range tests {
		r.Run(tt.name, func() {
			key := fmt.Sprintf("key_%s", id.New())
			defer r.cache.Delete(ctx, key)

			r.Require().NoError(r.cache.Set(ctx, key, tt.value, time.Minute))
			ptr, got := tt.get()
			r.Require().NoError(r.cache.Get(ctx, key, ptr))
			r.Equal(tt.value, got())
		})
	}
}

func (r *redisTestSuite) TestErrors() {
	ctx := context.TODO()
	key := fmt.Sprintf("key_%s", id.New())
	r.Require().NoError(r.cache.Set(ctx, key, "a string", time.Minute))
	defer r.cache.Delete(ctx, key)

	tests := []struct {
		name    string
		call    func() error
		wantErr error
		anyErr  bool
	}{
		{
			name:    "get empty key",
			call:    func() error { var v string; return r.cache.Get(ctx, "", &v) },
			wantErr: cache.ErrKeyInvalid,
		},
		{
			name:    "get missing key",
			call:    func() error { var v string; return r.cache.Get(ctx, "missing_"+id.New(), &v) },
			wantErr: cache.ErrNotFound,
		},
		{
			name:   "get into an incompatible type",
			call:   func() error { var v int; return r.cache.Get(ctx, key, &v) },
			anyErr: true,
		},
		{
			name:    "set empty key",
			call:    func() error { return r.cache.Set(ctx, "", "value", 0) },
			wantErr: cache.ErrKeyInvalid,
		},
		{
			name:    "set nil value",
			call:    func() error { return r.cache.Set(ctx, key, nil, 0) },
			wantErr: cache.ErrValueInvalid,
		},
		{
			name:   "set value that cannot be marshalled",
			call:   func() error { return r.cache.Set(ctx, key, make(chan int), 0) },
			anyErr: true,
		},
		{
			name:    "delete empty key",
			call:    func() error { return r.cache.Delete(ctx, "") },
			wantErr: cache.ErrKeyInvalid,
		},
		{
			name: "delete missing key",
			call: func() error { return r.cache.Delete(ctx, "missing_"+id.New()) },
		},
	}
	for _, tt := range tests {
		r.Run(tt.name, func() {
			err := tt.call()
			switch {
			case tt.anyErr:
				r.Error(err)
			case tt.wantErr != nil:
				r.ErrorIs(err, tt.wantErr)
			default:
				r.NoError(err)
			}
		})
	}
}

func (r *redisTestSuite) TestExpiration() {
	ctx := context.TODO()
	key := fmt.Sprintf("key_%s", id.New())
	defer r.cache.Delete(ctx, key)

	r.Require().NoError(r.cache.Set(ctx, key, "short lived", 100*time.Millisecond))
	var v string
	r.Require().NoError(r.cache.Get(ctx, key, &v))
	r.Equal("short lived", v)

	r.Eventually(func() bool {
		var v string
		return errors.Is(r.cache.Get(ctx, key, &v), cache.ErrNotFound)
	}, 5*time.Second, 20*time.Millisecond)
}

func (r *redisTestSuite) TestSetReplacesValueAndExpiration() {
	ctx := context.TODO()
	key := fmt.Sprintf("key_%s", id.New())
	defer r.cache.Delete(ctx, key)

	r.Require().NoError(r.cache.Set(ctx, key, "first", 100*time.Millisecond))
	// no expiration, the value is kept until deleted.
	r.Require().NoError(r.cache.Set(ctx, key, "second", 0))
	time.Sleep(200 * time.Millisecond)

	var v string
	r.Require().NoError(r.cache.Get(ctx, key, &v))
	r.Equal("second", v)
}

func (r *redisTestSuite) TestMultiGet() {
	ctx := context.TODO()
	key1 := fmt.Sprintf("key_%s", id.New())
	key2 := fmt.Sprintf("key_%s", id.New())
	keyString := fmt.Sprintf("key_%s", id.New())
	for k, v := range map[string]any{
		key1:      cacheItem{Name: "one", Count: 1},
		key2:      cacheItem{Name: "two", Count: 2},
		keyString: "not an item",
	} {
		r.Require().NoError(r.cache.Set(ctx, k, v, time.Minute))
		defer r.cache.Delete(ctx, k)
	}

	tests := []struct {
		name    string
		keys    []string
		value   func() any
		want    any
		wantErr bool
	}{
		{
			name:  "all keys found",
			keys:  []string{key1, key2},
			value: func() any { return &[]cacheItem{} },
			want:  &[]cacheItem{{Name: "one", Count: 1}, {Name: "two", Count: 2}},
		},
		{
			name:  "missing keys are skipped",
			keys:  []string{"missing_" + id.New(), key2},
			value: func() any { return &[]cacheItem{} },
			want:  &[]cacheItem{{Name: "two", Count: 2}},
		},
		{
			name:  "values of another type are skipped",
			keys:  []string{key1, keyString},
			value: func() any { return &[]cacheItem{} },
			want:  &[]cacheItem{{Name: "one", Count: 1}},
		},
		{
			name:  "values are appended",
			keys:  []string{key2},
			value: func() any { return &[]cacheItem{{Name: "existing"}} },
			want:  &[]cacheItem{{Name: "existing"}, {Name: "two", Count: 2}},
		},
		{
			name:  "no keys",
			keys:  nil,
			value: func() any { return &[]cacheItem{} },
			want:  &[]cacheItem{},
		},
		{
			name:    "value is not a pointer",
			keys:    []string{key1},
			value:   func() any { return []cacheItem{} },
			wantErr: true,
		},
		{
			name:    "value is not a pointer of slice",
			keys:    []string{key1},
			value:   func() any { return &cacheItem{} },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		r.Run(tt.name, func() {
			value := tt.value()
			err := r.cache.MultiGet(ctx, tt.keys, value)
			if tt.wantErr {
				r.Error(err)
				return
			}
			r.Require().NoError(err)
			r.Equal(tt.want, value)
		})
	}
}

func (r *redisTestSuite) TestClose() {
	addr, _ := os.LookupEnv("TESTINGREDIS_URL")
	c, err := New(&redis.Options{Addr: addr})
	r.Require().NoError(err)

	r.Require().NoError(c.Close())
	r.Error(c.Close(), "closing twice should fail")

	// using a closed cache should return an error, not panic.
	r.NotPanics(func() {
		var v string
		r.Error(c.Get(context.TODO(), "key", &v))
	}, "Get after Close")
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    *redis.Options
		wantErr bool
	}{
		{name: "missing options", opts: nil, wantErr: true},
		{name: "valid options", opts: &redis.Options{Addr: "127.0.0.1:0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			assert.NoError(t, c.Close())
		})
	}
}

func TestUnreachableRedis(t *testing.T) {
	// nothing is listening on this address.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	c, err := New(&redis.Options{Addr: addr, MaxRetries: -1, DialTimeout: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.TODO()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "get", call: func() error { var v string; return c.Get(ctx, "key", &v) }},
		{name: "set", call: func() error { return c.Set(ctx, "key", "value", 0) }},
		{name: "delete", call: func() error { return c.Delete(ctx, "key") }},
		{name: "multi get", call: func() error { var v []string; return c.MultiGet(ctx, []string{"key"}, &v) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			assert.Error(t, err)
			assert.NotErrorIs(t, err, cache.ErrNotFound)
		})
	}
}
