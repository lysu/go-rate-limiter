package ratelimiter_test

import (
	"testing"
	"time"

	"github.com/lysu/go-rate-limiter"
	"github.com/stretchr/testify/assert"
)

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
	rf := ratelimiter.MemoryLimiterCreate(ratelimiter.MemoryLimiterConfig{Interval: 1000 * time.Millisecond, MaxInInterval: 10})
	mr := rf()
	assertRateLimiter(mr, t)
}
