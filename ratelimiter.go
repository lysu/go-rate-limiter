package ratelimiter

import (
	"math"
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
	MinPeriod      int
	MaxInInterval  int
}

func setTimeout(f func(), interval time.Duration) *time.Timer {
	timer := time.NewTimer(interval)
	go func() {
		<-timer.C
		f()
	}()
	return timer
}

func clearTimeout(timer *time.Timer) {
	timer.Stop()
}

func mills2Nanos(m int64) int64 {
	return 1000000 * m
}

func nanos2Mills(n int64) int64 {
	return n / 1000000
}

// MemoryLimiterCreate use to create a limiter
func MemoryLimiterCreate(cfg MemoryLimiterConfig) CreateLimiter {
	return func() Allow {
		floodFlags := ttlcache.NewCache(cfg.Interval)
		timeoutTimers := make(map[string]*time.Timer)
		requestRecords := make(map[string][]int64)
		mutex := &sync.Mutex{}
		return func(id string) bool {
			_, hasFlood := floodFlags.Get(id)
			if cfg.FloodThreshold && hasFlood {
				return false
			}
			now := time.Now().UnixNano()
			before := now - cfg.Interval.Nanoseconds()

			timer, timerStarted := timeoutTimers[id]
			if timerStarted {
				clearTimeout(timer)
			}

			inIntervalReqs := inIntervalRequest(mutex, requestRecords, id, before)

			tooManyInInterval := len(inIntervalReqs) >= cfg.MaxInInterval

			isFlooded := cfg.FloodThreshold && tooManyInInterval && (len(inIntervalReqs) >= (3 * cfg.MaxInInterval))
			if isFlooded {
				floodFlags.Set(id, "xx")
			}

			lastReqPeriod := lastRequestPeriod(cfg, inIntervalReqs, now)

			var firstReq int64
			if len(inIntervalReqs) == 0 {
				firstReq = 0
			} else {
				firstReq = inIntervalReqs[0]
			}

			result := keepLimitTime(now, firstReq, tooManyInInterval, lastReqPeriod, cfg.MinPeriod, cfg.Interval)

			user, ok := requestRecords[id]
			if !ok {
				user = []int64{}
			}
			user = append(user, now)
			requestRecords[id] = user

			timeoutTimers[id] = setTimeout(func() {
				delete(requestRecords, id)
			}, cfg.Interval)

			return result <= 0
		}
	}
}

func keepLimitTime(now, firstReq int64, tooManyInInterval bool, timeSincelastReq int64, minPeriod int, interval time.Duration) int64 {
	if tooManyInInterval || ((minPeriod > 0 && timeSincelastReq > 0) && (timeSincelastReq < mills2Nanos(int64(minPeriod)))) {
		intervalLimitKeepTime := nanos2Mills((firstReq - now) + interval.Nanoseconds())
		var periodLimitKeepTime int64
		if minPeriod > 0 {
			periodLimitKeepTime = int64(minPeriod) - nanos2Mills(int64(timeSincelastReq))
		} else {
			periodLimitKeepTime = math.MaxInt64
		}
		if intervalLimitKeepTime >= periodLimitKeepTime {
			return periodLimitKeepTime
		}
		return intervalLimitKeepTime
	}
	return 0
}

func lastRequestPeriod(cfg MemoryLimiterConfig, userSet []int64, now int64) int64 {
	if cfg.MinPeriod == 0 || len(userSet) == 0 {
		return int64(0)
	}
	return now - userSet[len(userSet)-1]
}

func inIntervalRequest(mutex *sync.Mutex, storage map[string][]int64, key string, before int64) []int64 {
	mutex.Lock()
	defer mutex.Unlock()
	oldSet := storage[key]
	newSet := []int64{}
	for _, item := range oldSet {
		if item > before {
			newSet = append(newSet, item)
		}
	}
	storage[key] = newSet
	return newSet
}
