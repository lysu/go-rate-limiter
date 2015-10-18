package ratelimiter_test

import (
	"testing"
	"time"

	. "github.com/lysu/go-rate-limiter"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"sync"
)

type MemStore map[string]string

var mutex = &sync.Mutex{}

func (s MemStore) Get(ctx context.Context, key string) (string, error) {
	mutex.Lock()
	defer mutex.Unlock()
	return s[key], nil
}

func (s MemStore) Set(ctx context.Context, key, value string, ttl int64) error {
	mutex.Lock()
	defer mutex.Unlock()
	s[key] = value
	return nil
}

func (s MemStore) Delete(ctx context.Context, key string) error {
	mutex.Lock()
	defer mutex.Unlock()
	delete(s, key)
	return nil
}

func TestErrorLimiter(t *testing.T) {
	ctx := context.TODO()
	assert := assert.New(t)

	s := &MemStore{}

	l := CreateErrorLimiter("tn", s, 3, time.Millisecond*2000)

	pass, err := l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.True(pass)

	l.Count(ctx, "key1")

	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.True(pass)

	l.Count(ctx, "key1")
	l.Count(ctx, "key1")

	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.False(pass)

	time.Sleep(time.Millisecond * 2010)

	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.True(pass)

	l.Count(ctx, "key1")

	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.True(pass)

	l.Count(ctx, "key1")

	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.True(pass)
	l.Count(ctx, "key1")

	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.False(pass)

	l.Reset(ctx, "key1")
	pass, err = l.Allow(ctx, "key1")
	assert.NoError(err)
	assert.True(pass)

}
