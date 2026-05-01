package bloom

import (
	"context"
	"fmt"
	"log"

	"github.com/go-redis/redis/v8"
)

const bloomFilterKey = "short_url_bloom"

func InitBloom(rdb *redis.Client, capacity uint64, errorRate float64) error {
	ctx := context.Background()
	exists, err := rdb.Exists(ctx, bloomFilterKey).Result()
	if err != nil {
		return fmt.Errorf("check bloom exists failed %w", err)
	}
	if exists == 0 {
		_, err := rdb.Do(ctx, "BF.RESERVE", bloomFilterKey, errorRate, capacity, "EXPANSION", 2).Result()
		if err != nil {
			return fmt.Errorf("create bloom filter failed %w", err)
		}
		log.Println("Create bloom filter success")
	} else {
		log.Printf("bloom filter exists")
	}
	return nil
}

func Add(rdb *redis.Client, shortCode string) error {
	ctx := context.Background()
	_, err := rdb.Do(ctx, "BF.ADD", bloomFilterKey, shortCode).Result()
	return err
}

func AddBatch(rdb *redis.Client, shortCodes []string) error {
	if len(shortCodes) == 0 {
		return nil
	}
	ctx := context.Background()
	args := make([]interface{}, 0, len(shortCodes)+2)
	args = append(args, "BF.INSERT", bloomFilterKey, "ITEMS")
	for _, shortCode := range shortCodes {
		args = append(args, shortCode)
	}
	_, err := rdb.Do(ctx, args...).Result()
	return err
}

func Contains(rdb *redis.Client, shortCode string) (bool, error) {
	ctx := context.Background()
	result, err := rdb.Do(ctx, "BF.EXISTS", bloomFilterKey, shortCode).Result()
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
	GetAllShortKeys(batchSize, offset int) ([]string, error)
	GetTotalCount() (int, error)
}, batchSize int) error {
	log.Println("Startting bloom filter warmup...")
	total, err := repo.GetTotalCount()
	if err != nil {
		return fmt.Errorf("GetTotalCount failed with error %w", err)
	}
	if total == 0 {
		log.Println("No bloom filter warmup...")
		return nil
	}
	offset := 0
	addedCount := 0
	for {
		keys, err := repo.GetAllShortKeys(batchSize, offset)
		if err != nil {
			return fmt.Errorf("fetch short keys failed at offset %d: %w", offset, err)
		}
		if len(keys) == 0 {
			break
		}
		if err := AddBatch(rdb, keys); err != nil {
			return fmt.Errorf("add batch failed at offset %d: %w", offset, err)
		}
		addedCount += len(keys)
		log.Printf("Warmup progress: %d/%d keys added", addedCount, total)
		if len(keys) < batchSize {
			break
		}
		offset += batchSize
	}
	log.Printf("Bloom filter warmup completed: %d keys added", addedCount)
	return nil
}
