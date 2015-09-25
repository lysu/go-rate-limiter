package ratelimiter_test

import (
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/lysu/go-rate-limiter"
	"github.com/stretchr/testify/assert"
)

var redisPool = &redis.Pool{
	MaxIdle:     3,
	IdleTimeout: 240 * time.Second,
	Dial: func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", "127.0.0.1:6379")
		if err != nil {
			return nil, err
		}
		return c, err
	},
	TestOnBorrow: func(c redis.Conn, t time.Time) error {
		_, err := c.Do("PING")
		return err
	},
}

func clearUpRedis() {
	conn := redisPool.Get()
	defer conn.Close()
	conn.Do("DEL", "rl-key1")
	conn.Do("DEL", "rl-key2")
}

func size(key string) int {
	conn := redisPool.Get()
	defer conn.Close()
	r, err := conn.Do("ZCARD", key)
	if err != nil {
		panic("fetch size failure")
	}
	ri, _ := redis.Int(r, err)
	return ri
}

func assertRateLimiter(r func(key string) bool, t *testing.T) {
	assert := assert.New(t)

	key1 := "key1"
	allowCount := 0
	for i := 0; i < 100; i++ {
		if r(key1) {
			allowCount++
		}
	}
	assert.Equal(10, allowCount, "key1 1st round not limit")

	key2 := "key2"
	allowCount = 0
	for i := 0; i < 100; i++ {
		if r(key2) {
			allowCount++
		}
	}
	assert.Equal(10, allowCount, "key2 1st round not limit")

	time.Sleep(1050 * time.Millisecond)

	allowCount = 0
	for i := 0; i < 100; i++ {
		if r(key1) {
			allowCount++
		}
	}
	assert.Equal(10, allowCount, "key1 2nd round not limit")

	allowCount = 0
	for i := 0; i < 100; i++ {
		if r(key2) {
			allowCount++
		}
	}
	assert.Equal(10, allowCount, "key2 2nd round not limit")
}

func TestMemoryLimiter(t *testing.T) {
	rf := ratelimiter.MemoryLimiterCreate(ratelimiter.LimiterConfig{Interval: 1000 * time.Millisecond, MaxInInterval: 10})
	mr := rf()
	assertRateLimiter(mr, t)
}

func TestRedisLimiter(t *testing.T) {
	clearUpRedis()
	rf := ratelimiter.RedisLimiterCreate(ratelimiter.LimiterConfig{RedisPool: redisPool, Interval: 1000 * time.Millisecond, MaxInInterval: 10})
	rr := rf()
	assertRateLimiter(rr, t)
	assert.Equal(t, 100, size("rl-key1"))
}

func TestFloodThreshold(t *testing.T) {
	clearUpRedis()
	rf := ratelimiter.RedisLimiterCreate(ratelimiter.LimiterConfig{RedisPool: redisPool, Interval: 1000 * time.Millisecond, MaxInInterval: 10, FloodThreshold: 5})
	rr := rf()
	assertRateLimiter(rr, t)
	assert.Equal(t, 51, size("rl-key1"))
	time.Sleep(1500 * time.Millisecond)
	assertRateLimiter(rr, t)
}

func TestMinPeriod(t *testing.T) {
	clearUpRedis()
	rf := ratelimiter.RedisLimiterCreate(ratelimiter.LimiterConfig{RedisPool: redisPool, Interval: 1000 * time.Millisecond, MinPeriod: 10 * time.Millisecond, MaxInInterval: 10, FloodThreshold: 5})
	allow := rf()
	assert.True(t, allow("key1"))
	assert.False(t, allow("key1"))
	assert.True(t, allow("key2"))
	time.Sleep(15 * time.Millisecond)
	assert.True(t, allow("key1"))
	assert.False(t, allow("key1"))
	assert.True(t, allow("key2"))
}
