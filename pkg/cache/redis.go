package cache

import (
	"context"
	"short-url-service/pkg/bloom"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(addr, password string, db int) *RedisClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: 100,
	})
	return &RedisClient{client: rdb}
}

func (r *RedisClient) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisClient) Incr(ctx context.Context, key string) *redis.IntCmd {
	return r.client.Incr(ctx, key)
}

func (r *RedisClient) Raw() *redis.Client {
	return r.client
}

func (r *RedisClient) SetWithBloom(ctx context.Context, shortCode, longURL string, expiration time.Duration) error {
	pipe := r.client.Pipeline()
	// Bloom Add
	pipe.Do(ctx, "BF.ADD", bloom.BloomFilterKey, shortCode) // 需要导出 bloomFilterKey
	// Cache Set
	pipe.Set(ctx, shortCode, longURL, expiration)
	_, err := pipe.Exec(ctx)
	return err
}
