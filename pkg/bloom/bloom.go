package bloom

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-redis/redis/v8"
)

const (
	BloomFilterKey  = "short_url_bloom"
	bloomReserveCmd = "BF.RESERVE"
	bloomAddCmd     = "BF.ADD"
	bloomInsertCmd  = "BF.INSERT"
	bloomExistsCmd  = "BF.EXISTS"
	bloomExpansion  = 2
)

func InitBloom(rdb *redis.Client, capacity uint64, errorRate float64) error {
	ctx := context.Background()
	exists, err := rdb.Exists(ctx, BloomFilterKey).Result()
	if err != nil {
		return fmt.Errorf("check bloom exists failed %w", err)
	}
	if exists == 0 {
		_, err := rdb.Do(ctx, bloomReserveCmd, BloomFilterKey, errorRate, capacity, "EXPANSION", bloomExpansion).Result()
		if err != nil {
			return fmt.Errorf("create bloom filter failed %w", err)
		}
		slog.InfoContext(context.Background(), "Create bloom filter success")
	} else {
		slog.InfoContext(context.Background(), "bloom filter exists")
	}
	return nil
}

func Add(rdb *redis.Client, shortCode string) error {
	ctx := context.Background()
	_, err := rdb.Do(ctx, bloomAddCmd, BloomFilterKey, shortCode).Result()
	return err
}

func AddBatch(rdb *redis.Client, shortCodes []string) error {
	if len(shortCodes) == 0 {
		return nil
	}
	ctx := context.Background()
	args := make([]interface{}, 0, len(shortCodes)+2)
	args = append(args, bloomInsertCmd, BloomFilterKey, "ITEMS")
	for _, shortCode := range shortCodes {
		args = append(args, shortCode)
	}
	_, err := rdb.Do(ctx, args...).Result()
	return err
}

func Contains(rdb *redis.Client, shortCode string) (bool, error) {
	ctx := context.Background()
	result, err := rdb.Do(ctx, bloomExistsCmd, BloomFilterKey, shortCode).Result()
	if err != nil {
		return false, err
	}
	exists, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected result type from BF.EXISTS")
	}
	return exists == 1, nil
}

func Warmup(rdb *redis.Client, repo interface {
	GetAllShortKeysCursor(batchSize int, lastID int64) ([]string, int64, error)
}, batchSize int, ctx context.Context) error {
	slog.InfoContext(ctx, "Starting bloom filter warmup...")

	var lastID int64 = 0
	addedCount := 0
	for {
		keys, newLastID, err := repo.GetAllShortKeysCursor(batchSize, lastID)
		if err != nil {
			return fmt.Errorf("fetch short keys failed: %w", err)
		}
		if len(keys) == 0 {
			break
		}
		if err := AddBatch(rdb, keys); err != nil {
			return fmt.Errorf("add batch failed: %w", err)
		}
		addedCount += len(keys)
		slog.InfoContext(ctx, "Bloom filter warmup progress",
			"added", addedCount,
			"lastID", newLastID,
		)
		if len(keys) < batchSize {
			break // 最后一批
		}
		lastID = newLastID
	}
	slog.InfoContext(ctx, "Bloom filter warmup completed", "added", addedCount)
	return nil
}
