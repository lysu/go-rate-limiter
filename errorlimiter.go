package ratelimiter

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/net/context"
)

// ErrorLimiter use to limiter when error occur N time
type ErrorLimiter interface {
	// Allow is core function judge whether request pass
	Allow(ctx context.Context, key string) (bool, error)
	// Count count error
	Count(ctx context.Context, key string) error
	// Reset reset error count
	Reset(ctx context.Context, key string) error
}

// KvStore present kv store operation
type KvStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl int64) error
	Delete(ctx context.Context, key string) error
}

type reqRecord struct {
	LastTime int64 `json:"lastTime"`
	CurCount int64 `json:"curCount"`
}

// CreateErrorLimiter use to create error limiter
func CreateErrorLimiter(ns string, store KvStore, maxErrors int64, forbiddenInterval time.Duration) ErrorLimiter {
	return &errorLimitImp{
		namespace:         ns,
		store:             store,
		maxErrors:         maxErrors,
		forbiddenInterval: forbiddenInterval,
	}
}

type errorLimitImp struct {
	namespace         string
	store             KvStore
	maxErrors         int64
	forbiddenInterval time.Duration
}

func (l *errorLimitImp) Allow(ctx context.Context, key string) (bool, error) {

	now := time.Now().UnixNano()

	var err error
	pass := true
	cacheKey := fmt.Sprintf("%s-%s", l.namespace, key)

	countRecord, err := l.restoreCountRecord(ctx, cacheKey)
	if err != nil {
		return false, err
	}

	if now-countRecord.LastTime < l.forbiddenInterval.Nanoseconds() {
		if countRecord.CurCount >= l.maxErrors {
			pass = false
		}
	} else {
		l.Reset(ctx, key)
	}

	return pass, err

}

func (l *errorLimitImp) Count(ctx context.Context, key string) error {

	now := time.Now().UnixNano()

	var err error
	cacheKey := fmt.Sprintf("%s-%s", l.namespace, key)

	countRecord, err := l.restoreCountRecord(ctx, cacheKey)
	if err != nil {
		return err
	}

	newRecord := &reqRecord{LastTime: now, CurCount: countRecord.CurCount + 1}

	l.storeContRecord(ctx, cacheKey, newRecord)
	if err != nil {
		return err
	}

	return nil
}

func (l *errorLimitImp) Reset(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf("%s-%s", l.namespace, key)
	return l.store.Delete(ctx, cacheKey)
}

func (l *errorLimitImp) restoreCountRecord(ctx context.Context, key string) (*reqRecord, error) {
	oldRecord, err := l.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if oldRecord == "" {
		return &reqRecord{
			LastTime: time.Now().UnixNano(),
			CurCount: int64(0),
		}, nil
	}
	cr := &reqRecord{}
	if err := json.Unmarshal([]byte(oldRecord), &cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (l *errorLimitImp) storeContRecord(ctx context.Context, cacheKey string, newReqRecord *reqRecord) error {
	newCountData, err := json.Marshal(newReqRecord)
	if err != nil {
		return errors.New("json marshal failed")
	}
	err = l.store.Set(ctx, cacheKey, string(newCountData), int64(l.forbiddenInterval.Seconds()))
	if err != nil {
		return errors.New("store set failed")
	}
	return nil
}
