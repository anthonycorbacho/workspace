package redis

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/cache"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
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
