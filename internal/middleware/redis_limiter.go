package middleware

import (
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/go-redis/redis_rate/v9"
)

// RedisLimiter 基于 Redis GCRA 的分布式限流器
type RedisLimiter struct {
	limiter *redis_rate.Limiter
	rate    int // 每秒允许请求数
}

func NewRedisLimiter(rdb *redis.Client, ratePerSecond int) *RedisLimiter {
	return &RedisLimiter{
		limiter: redis_rate.NewLimiter(rdb),
		rate:    ratePerSecond,
	}
}

func (r *RedisLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := getRealIp(req) // 复用原有的 getRealIp 函数
		res, err := r.limiter.Allow(req.Context(), ip, redis_rate.PerSecond(r.rate))
		if err != nil {
			// 降级：Redis 故障时记录日志并放行（可用性优先）
			next.ServeHTTP(w, req)
			return
		}
		if res.Allowed == 0 {
			w.Header().Set("X-RateLimit-Limit", "20")
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, req)
	})
}
