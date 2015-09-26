# go-rate-limiter

RateLimiter for Go, use TokenBucket algorithm, base on both Memory and Redis.

It's transformed from [clj-rate-limiter](https://github.com/killme2008/clj-rate-limiter) in clojure

and it use [functional-options-style](http://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis) API.

## Usage

    go get github.com/lysu/go-rate-limiter

## Bucket

Create an in-memory rate limiter with maximum 100 requsts in 1 seconds:

    import (
      limiter "github.com/lysu/go-rate-limiter"
    )

    allow := limiter.RateLimiter(limiter.UseMemory(), limiter.BucketLimit(1*time,Second, 100))()
    fmt.Println(allow("key1"))
    fmt.Println(allow("key2"))

The `limiter.UseMemory()` let limiter use memory storge, `limiter.BucketLimit(1*time,Second, 100)` sets the time window size and the maximum requests in the window. After the limiter is created, you can use `allow("key")` to test if the request can be passed.The string key is present the type of the request.A group requests of the same key are rate limited by the limiter.

In a cluster, you can choose use redis as backend storage, and we choose `redigo` as reids client

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
    allow := limiter.RateLimiter(limiter.UseRedis(redisPool), limiter.BucketLimit(1*time,Second, 100))()
    fmt.Println(allow("key1"))
    fmt.Println(allow("key2"))

## Period

You can set min period between two requests, by use `MinPeriod` option.

    allow := limiter.RateLimiter(limiter.UseRedis(redisPool), limiter.MinPeriod(10*time.Millisecond))()
    fmt.Println(allow("key1"))
    fmt.Println(allow("key2"))

## Flood Requests

The rate limiter will still add the request to backend storage even if the requests are too many. It may consume too much memory in backend storage,so i provide a option `FloodThreshold` for such situation.

If you set `FloodThreshold`,when current requests number in storage is greater or equalt to `FloodThreshold * MaxInInterval`, then the limiter will not add the new request to backend storage until next time window.

    allow := limiter.RateLimiter(limiter.UseRedis(redisPool), limiter.BucketLimit(1000*time.Millisecond, 10), limiter.FloodThreshold(5))()

## More

more usage can see [UnitTest](https://github.com/lysu/go-rate-limiter/blob/master/ratelimite_test.go)
