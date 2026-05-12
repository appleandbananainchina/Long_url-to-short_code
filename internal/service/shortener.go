package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"short-url-service/internal/repository"
	"short-url-service/pkg/bloom"
	"short-url-service/pkg/breaker"
	"short-url-service/pkg/cache"
	"short-url-service/pkg/idgen"
	"short-url-service/pkg/metrics"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/sync/singleflight"
)

const (
	base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	redisExpire = 24 * time.Hour // 缓存有效期1天
)

var ErrConflict = errors.New("customkey already exist")
var ErrNotFound = errors.New("short_url not found")
var sf singleflight.Group

type ShortenerService struct {
	idGen     *idgen.Snowflake
	mysqlRepo *repository.MySQLRepo
	redisCli  *cache.RedisClient
}

func NewShortenerService(machineID int64, mysqlRepo *repository.MySQLRepo, redisCli *cache.RedisClient) *ShortenerService {
	return &ShortenerService{
		idGen:     idgen.NewSnowflake(machineID),
		mysqlRepo: mysqlRepo,
		redisCli:  redisCli,
	}
}

// 将整数ID编码为base62短码
func encodeBase62(id int64) string {
	if id == 0 {
		return string(base62Chars[0])
	}
	var result []byte
	for id > 0 {
		rem := id % 62
		result = append([]byte{base62Chars[rem]}, result...)
		id = id / 62
	}
	return string(result)
}

// 生成短码（由唯一ID编码而来）
func (s *ShortenerService) generateShortCode() string {
	id := s.idGen.NextID()
	return encodeBase62(id)
}

// Shorten 接收长URL，生成短码并存储
func (s *ShortenerService) Shorten(ctx context.Context, longURL, customKey string) (string, error) {
	var shortCode string
	if customKey == "" {
		shortCode = s.generateShortCode()
	} else {
		url, err := s.GetLongURL(ctx, customKey)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				shortCode = customKey
			} else {
				return "", fmt.Errorf("component error: %w", err)
			}
		} else {
			if url != longURL {
				return "", fmt.Errorf("customkey already exist")
			} else {
				return customKey, nil
			}
		}
	}

	//关于写入顺序，在这里先写布隆过滤器。避免写入失败导致的数据库中存在，而布隆过滤器返回不存在的假阴性行为（导致自定义短链的假冲突，使某个短链完全不可用）。

	//储存到布隆过略器
	if err := bloom.Add(s.redisCli.Raw(), shortCode); err != nil {
		for retry := 0; retry < 3; retry++ {
			if err = bloom.Add(s.redisCli.Raw(), shortCode); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond << retry)
		}
		if err != nil {
			return "", fmt.Errorf("failed to add to redis: %w", err)
		}
	}

	// 存储到MySQL（持久化）
	const maxRetries = 3
	var saveErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		saveErr = s.mysqlRepo.SaveIdempotent(ctx, shortCode, longURL)
		if saveErr == nil {
			break
		}
		if errors.Is(saveErr, repository.ErrDuplicateKey) {
			// 真冲突（不同 longURL），不重试直接返回
			return "", ErrConflict
		}
		// 其他错误（网络、超时等）重试
		if attempt == maxRetries {
			return "", fmt.Errorf("failed to save to mysql after %d attempts: %w", maxRetries, saveErr)
		}
		backoff := time.Duration(1<<attempt) * 50 * time.Millisecond // 指数退避: 100ms, 200ms, 400ms
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	// 写入Redis缓存
	const maxRedisRetries = 2
	var setErr error
	for attempt := 1; attempt <= maxRedisRetries; attempt++ {
		setErr = s.redisCli.Set(ctx, shortCode, longURL, redisExpire)
		if setErr == nil {
			break
		}
		if attempt == maxRedisRetries {
			slog.WarnContext(ctx, "redis cache set failed after retries", "error", setErr)
			// 不返回错误，只记录日志（缓存不影响主流程）
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return shortCode, nil
}

// GetLongURL 根据短码获取长URL，优先读缓存，未命中则查MySQL并回写缓存
func (s *ShortenerService) GetLongURL(ctx context.Context, shortCode string) (string, error) {
	// 布隆过滤器快速判断
	exists, err := breaker.BloomBreaker.Do(ctx, func() (bool, error) {
		return bloom.Contains(s.redisCli.Raw(), shortCode)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			slog.WarnContext(ctx, "bloom contains error, fallback to db", "error", err)
		} else {
			slog.Warn("bloom call error", "err", err)
		}
	} else if !exists {
		metrics.BloomFilterMisses.Inc()
		return "", ErrNotFound
	}
	metrics.BloomFilterHits.Inc()

	// 查缓存
	longURL, err := breaker.RedisCacheBreaker.Do(ctx, func() (string, error) {
		return s.redisCli.Get(ctx, shortCode)
	})
	if err == nil {
		return longURL, nil
	}
	if errors.Is(err, gobreaker.ErrOpenState) {
		slog.WarnContext(ctx, "redis cache breaker open, skip cache")
	} else {
		slog.DebugContext(ctx, "cache miss or error", "error", err)
	}

	ch := sf.DoChan(shortCode, func() (interface{}, error) {
		longURL, err := s.redisCli.Get(ctx, shortCode)
		if err == nil && longURL != "" {
			return longURL, nil
		}
		longURL, err = s.mysqlRepo.GetWithBreaker(ctx, shortCode)
		if err != nil {
			return "", err
		}
		if longURL == "" {
			return "", ErrNotFound // 未找到
		}
		_ = s.redisCli.Set(ctx, shortCode, longURL, redisExpire)
		return longURL, nil
	})
	select {
	case res := <-ch: // 等待真正执行的结果
		if res.Err != nil {
			return "", res.Err
		}
		return res.Val.(string), nil
	case <-ctx.Done(): // 超时或者主动取消
		return "", ctx.Err()
	}
}

/**func (s *ShortenerService) SetIncr(ctx context.Context, application, key string) error {
	err := s.redisCli.Incr(ctx, fmt.Sprintf("stats:%s:%s", application, key)).Err()
	if err != nil {
		return err
	}
	return nil
}**/
