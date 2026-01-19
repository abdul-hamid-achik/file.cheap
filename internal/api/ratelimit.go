package api

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter implements a sliding window rate limiter using Redis
type RedisRateLimiter struct {
	client *redis.Client
	rate   int
	window time.Duration
	prefix string
}

// NewRedisRateLimiter creates a new Redis-backed rate limiter
func NewRedisRateLimiter(client *redis.Client, rate int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		rate:   rate,
		window: window,
		prefix: "ratelimit:",
	}
}

// Allow checks if a request should be allowed for the given key
// Returns true if allowed, false if rate limited
func (rl *RedisRateLimiter) Allow(ctx context.Context, key string) bool {
	if rl.client == nil {
		return true
	}

	now := time.Now().UnixNano()
	windowStart := now - int64(rl.window)
	redisKey := fmt.Sprintf("%s%s", rl.prefix, key)

	pipe := rl.client.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))

	// Add current request timestamp
	pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(now), Member: now})

	// Count requests in window
	countCmd := pipe.ZCard(ctx, redisKey)

	// Set TTL for the key
	pipe.Expire(ctx, redisKey, rl.window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		// Fail open - allow request if Redis fails
		return true
	}

	count := countCmd.Val()
	return count <= int64(rl.rate)
}

// AllowN checks if n requests should be allowed
func (rl *RedisRateLimiter) AllowN(ctx context.Context, key string, n int) bool {
	if rl.client == nil {
		return true
	}

	now := time.Now().UnixNano()
	windowStart := now - int64(rl.window)
	redisKey := fmt.Sprintf("%s%s", rl.prefix, key)

	pipe := rl.client.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))

	// Count current requests in window
	countCmd := pipe.ZCard(ctx, redisKey)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return true
	}

	currentCount := countCmd.Val()
	return int(currentCount)+n <= rl.rate
}

// Remaining returns the number of requests remaining in the current window
func (rl *RedisRateLimiter) Remaining(ctx context.Context, key string) int {
	if rl.client == nil {
		return rl.rate
	}

	now := time.Now().UnixNano()
	windowStart := now - int64(rl.window)
	redisKey := fmt.Sprintf("%s%s", rl.prefix, key)

	// Remove old entries and count
	rl.client.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))
	count, err := rl.client.ZCard(ctx, redisKey).Result()
	if err != nil {
		return rl.rate
	}

	remaining := rl.rate - int(count)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Reset resets the rate limit for a key
func (rl *RedisRateLimiter) Reset(ctx context.Context, key string) error {
	if rl.client == nil {
		return nil
	}

	redisKey := fmt.Sprintf("%s%s", rl.prefix, key)
	return rl.client.Del(ctx, redisKey).Err()
}

// HybridRateLimiter combines Redis and in-memory rate limiting
// Uses Redis when available, falls back to in-memory
type HybridRateLimiter struct {
	redis    *RedisRateLimiter
	inMemory *RateLimiter
}

// NewHybridRateLimiter creates a rate limiter that uses Redis when available
func NewHybridRateLimiter(redisClient *redis.Client, rate, burst int) *HybridRateLimiter {
	var redisRL *RedisRateLimiter
	if redisClient != nil {
		redisRL = NewRedisRateLimiter(redisClient, rate, time.Second)
	}

	return &HybridRateLimiter{
		redis:    redisRL,
		inMemory: NewRateLimiter(rate, burst),
	}
}

// Allow checks if a request should be allowed
func (hl *HybridRateLimiter) Allow(ctx context.Context, key string) bool {
	if hl.redis != nil && hl.redis.client != nil {
		// Try Redis first
		if err := hl.redis.client.Ping(ctx).Err(); err == nil {
			return hl.redis.Allow(ctx, key)
		}
	}
	// Fall back to in-memory
	return hl.inMemory.Allow(key)
}

// Stop stops the in-memory cleanup goroutine
func (hl *HybridRateLimiter) Stop() {
	hl.inMemory.Stop()
}
