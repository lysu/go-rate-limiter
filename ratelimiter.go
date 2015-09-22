package ratelimiter

import (
	"fmt"
	"sync"
	"time"

	"github.com/wunderlist/ttlcache"
)

// Allow to judge special op key whether allowed to process
type Allow func(key string) bool

// CreateLimiter use to create new limiter
type CreateLimiter func() Allow

// MemoryLimiterConfig use to config memory limiter
type MemoryLimiterConfig struct {
	FloodThreshold bool
	Interval       time.Duration
	MinDifference  int
	MaxInternal    int
}

// MemoryLimiterCreate use to create a limiter
func MemoryLimiterCreate(cfg MemoryLimiterConfig) CreateLimiter {
	return func() Allow {
		floodCache := ttlcache.NewCache(cfg.Interval)
		timeouts := make(map[time.Duration]interface{})
		storage := make(map[string]map[int64]struct{})
		mutex := &sync.Mutex{}
		return func(key string) bool {
			now := time.Now().UnixNano()
			k := fmt.Sprintf("%s-%s", "l", key)
			before := now - cfg.Interval.Nanoseconds()

			_, exist := floodCache.Get(key)
			if cfg.FloodThreshold && exist {
				return false
			}

			userSet := takeUserSet(storage, key, before)

			tooManyInInterval := len(userSet) > cfg.MaxInternal

			floodReq := cfg.FloodThreshold && tooManyInInterval && (len(userSet) >= (3 * cfg.MaxInternal))

			timeSinceLastReq := now - last(userSet)

			return false
		}
	}
}

func takeUserSet(storage map[string]map[int64]struct{}, key string, before int64) map[int64]struct{} {
	oldSet := storage[key]
	newSet := make(map[int64]struct{})
	for k, v := range oldSet {
		if k > before {
			newSet[k] = v
		}
	}
	storage[key] = newSet
	return newSet
}
