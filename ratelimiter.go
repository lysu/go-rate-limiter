package ratelimiter

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/wunderlist/ttlcache"
)

// LimiterType present type of limiter
type LimiterType int

const (
	// Redis base limiter
	Redis LimiterType = iota
	// Memory base limiter
	Memory
)

// Allow to judge special op key whether allowed to process
type Allow func(key string) bool

// CreateLimiter use to create new limiter
type CreateLimiter func() Allow

// limiterConfig use to config redis limiter
type limiterConfig struct {
	LimiterType    LimiterType
	FloodThreshold int
	Interval       time.Duration
	MinPeriod      time.Duration
	MaxInInterval  int
	RedisPool      *redis.Pool
}

func defaultConfig() limiterConfig {
	return limiterConfig{Interval: 1000 * time.Millisecond}
}

// UseMemory option control use memory base limiter
func UseMemory() func(config *limiterConfig) {
	return func(config *limiterConfig) {
		config.LimiterType = Memory
	}
}

// UseRedis option control use redis base limiter
// redisPool supply redis config now we use redigo
func UseRedis(redisPool *redis.Pool) func(config *limiterConfig) {
	return func(config *limiterConfig) {
		config.LimiterType = Redis
		config.RedisPool = redisPool
	}
}

// FloodThreshold protect backend when flood-like request came
// The rate limiter will still add the request to backend storage even if the requests are too many.
// It may consume too much memory in backend storage, so this option for it
func FloodThreshold(floodThreshold int) func(config *limiterConfig) {
	return func(config *limiterConfig) {
		config.FloodThreshold = floodThreshold
	}
}

// MinPeriod control the min period between two requests
func MinPeriod(minPeriod time.Duration) func(config *limiterConfig) {
	return func(config *limiterConfig) {
		config.MinPeriod = minPeriod
	}
}

// BucketLimit control use BucketToken limit logic
func BucketLimit(interval time.Duration, maxInInterval int) func(config *limiterConfig) {
	return func(config *limiterConfig) {
		config.Interval = interval
		config.MaxInInterval = maxInInterval
	}
}

// RateLimiter is use-entry of RateLimiter
func RateLimiter(optfs ...func(config *limiterConfig)) CreateLimiter {
	cfg := defaultConfig()
	for _, optf := range optfs {
		optf(&cfg)
	}
	switch cfg.LimiterType {
	case Redis:
		return redisLimiterCreate(cfg)
	case Memory:
		return memoryLimiterCreate(cfg)
	}
	return func() Allow {
		return func(key string) bool {
			return false
		}
	}
}

// memoryLimiterCreate use to create a limiter base on memory
func memoryLimiterCreate(cfg limiterConfig) CreateLimiter {
	return func() Allow {
		floodFlags := ttlcache.NewCache(cfg.Interval)
		timeoutTimers := make(map[string]*time.Timer)
		requestRecords := make(map[string][]int64)
		mutex := &sync.Mutex{}
		return func(id string) bool {
			_, hasFlood := floodFlags.Get(id)
			if cfg.FloodThreshold > 0 && hasFlood {
				return false
			}
			now := time.Now().UnixNano()
			before := now - cfg.Interval.Nanoseconds()

			timer, timerStarted := timeoutTimers[id]
			if timerStarted {
				timer.Stop()
			}

			inIntervalReqs := inIntervalRequest(mutex, requestRecords, id, before)

			tooManyInInterval := len(inIntervalReqs) >= cfg.MaxInInterval

			isFlooded := cfg.FloodThreshold > 0 && tooManyInInterval && (len(inIntervalReqs) >= (cfg.FloodThreshold * cfg.MaxInInterval))
			if isFlooded {
				floodFlags.Set(id, "xx")
			}

			lastReqPeriod := lastRequestPeriod(cfg.MinPeriod, inIntervalReqs, now)

			var firstReq int64
			if len(inIntervalReqs) == 0 {
				firstReq = 0
			} else {
				firstReq = inIntervalReqs[0]
			}

			waitOpenTime := waitOpenTime(now, firstReq, tooManyInInterval, lastReqPeriod, cfg.MinPeriod, cfg.Interval)

			user, ok := requestRecords[id]
			if !ok {
				user = []int64{}
			}
			user = append(user, now)
			requestRecords[id] = user

			timeoutTimers[id] = setTimeout(func() {
				delete(requestRecords, id)
			}, cfg.Interval)

			return waitOpenTime <= 0
		}
	}
}

// redisLimiterCreate use to create limiter base on redis
func redisLimiterCreate(cfg limiterConfig) CreateLimiter {
	return func() Allow {
		floodFlags := ttlcache.NewCache(cfg.Interval)
		return func(id string) bool {
			_, hasFlood := floodFlags.Get(id)
			if cfg.FloodThreshold > 0 && hasFlood {
				return false
			}
			now := time.Now().UnixNano()
			key := fmt.Sprintf("%s-%s", "rl", id)
			before := now - cfg.Interval.Nanoseconds()

			total, firstReq, lastReq, err := checkRedis(cfg.RedisPool, key, before, now, cfg.Interval)
			if err != nil {
				return true
			}
			tooManyInInterval := total >= cfg.MaxInInterval

			isFlooded := cfg.FloodThreshold > 0 && tooManyInInterval && (total >= (cfg.FloodThreshold * cfg.MaxInInterval))
			if isFlooded {
				floodFlags.Set(id, "xx")
			}
			var lastReqPeriod int64
			if cfg.MinPeriod > 0 && lastReq > 0 {
				lastReqPeriod = now - lastReq
			}

			waitOpenTime := waitOpenTime(now, firstReq, tooManyInInterval, lastReqPeriod, cfg.MinPeriod, cfg.Interval)

			return waitOpenTime <= 0
		}
	}
}

// checkRedis check status and add current ts to redis
func checkRedis(redisPool *redis.Pool, key string, before, now int64, interval time.Duration) (int, int64, int64, error) {
	c := redisPool.Get()
	defer func() {
		if c != nil {
			c.Close()
		}
	}()
	c.Send("MULTI")
	c.Send("ZREMRANGEBYSCORE", key, 0, before)
	c.Send("ZCARD", key)
	c.Send("ZRANGEBYSCORE", key, "-inf", "+inf", "LIMIT", 0, 1)
	c.Send("ZREVRANGEBYSCORE", key, "+inf", "-inf", "LIMIT", 0, 1)
	c.Send("ZADD", key, now, now)
	c.Send("EXPIRE", key, interval.Seconds())
	r, err := c.Do("EXEC")
	if err != nil {
		return 0, 0, 0, err
	}
	rs, cast := r.([]interface{})
	if !cast {
		return 0, 0, 0, fmt.Errorf("cast error")
	}
	total, err := redis.Int(rs[1], nil)
	if err != nil {
		return 0, 0, 0, err
	}
	fa, err := redis.Strings(rs[2], nil)
	if err != nil {
		return 0, 0, 0, err
	}
	la, err := redis.Strings(rs[3], nil)
	if err != nil {
		return 0, 0, 0, err
	}
	var (
		firstReq int64
		lastReq  int64
	)
	firstReq, err = parseFirst(fa)
	if err != nil {
		return 0, 0, 0, err
	}
	lastReq, err = parseFirst(la)
	if err != nil {
		return 0, 0, 0, err
	}
	return total, firstReq, lastReq, nil
}

func setTimeout(f func(), interval time.Duration) *time.Timer {
	timer := time.NewTimer(interval)
	go func() {
		<-timer.C
		f()
	}()
	return timer
}

func nanos2Mills(n int64) int64 {
	return n / 1000000
}

func parseFirst(ss []string) (int64, error) {
	if len(ss) == 0 {
		return 0, nil
	}
	num, err := strconv.ParseInt(ss[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse int failure: %s", ss[0])
	}
	return num, nil
}

func waitOpenTime(now, firstReq int64, tooManyInInterval bool, timeSincelastReq int64, minPeriod, interval time.Duration) int64 {
	if tooManyInInterval || ((minPeriod > 0 && timeSincelastReq > 0) && (timeSincelastReq < minPeriod.Nanoseconds())) {
		intervalLimitKeepTime := nanos2Mills((firstReq - now) + interval.Nanoseconds())
		var periodLimitKeepTime int64
		if minPeriod > 0 {
			periodLimitKeepTime = nanos2Mills(minPeriod.Nanoseconds() - timeSincelastReq)
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

func lastRequestPeriod(minPeriod time.Duration, userSet []int64, now int64) int64 {
	if minPeriod == 0 || len(userSet) == 0 {
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
