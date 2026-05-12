package breaker

import "time"

var (
	// BloomBreaker 用于布隆过滤器操作
	BloomBreaker = New[bool](Config{
		Name:                "bloom-filter",
		MaxRequests:         3,
		Timeout:             10 * time.Second,
		ConsecutiveFailures: 999999,
	})

	// RedisCacheBreaker 用于 Redis 缓存读取
	RedisCacheBreaker = New[string](Config{
		Name:                "redis-cache",
		MaxRequests:         5,
		Timeout:             5 * time.Second,
		ConsecutiveFailures: 999999,
	})

	// MySQLBreaker 用于 MySQL 查询
	MySQLBreaker = New[string](Config{
		Name:                "mysql-db",
		MaxRequests:         2,
		Timeout:             15 * time.Second,
		ConsecutiveFailures: 999999,
	})
)
