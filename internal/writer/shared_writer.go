// internal/writer/shard_writer.go
package writer

import (
	"context"
	"log/slog"
	"short-url-service/internal/repository"
	"sync"
	"time"
)

type ShardWriter struct {
	id            int
	taskCh        chan *WriteTask // 接收来自 distributor 的任务
	batchSize     int
	flushInterval time.Duration
	mysqlRepo     *repository.MySQLRepo
	wg            sync.WaitGroup
	stopCh        chan struct{}
}

func NewShardWriter(id, batchSize int, flushInterval time.Duration, mysqlRepo *repository.MySQLRepo) *ShardWriter {
	return &ShardWriter{
		id:            id,
		taskCh:        make(chan *WriteTask, 2048),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		mysqlRepo:     mysqlRepo,
		stopCh:        make(chan struct{}),
	}
}

func (sw *ShardWriter) Start() {
	sw.wg.Add(1)
	go sw.run()
}

func (sw *ShardWriter) run() {
	defer sw.wg.Done()
	ticker := time.NewTicker(sw.flushInterval)
	defer ticker.Stop()

	batch := make([]*WriteTask, 0, sw.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		sw.flushBatch(batch)
		for _, t := range batch {
			ReleaseTask(t)
		}
		batch = batch[:0]
	}

	for {
		select {
		case task, ok := <-sw.taskCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, task)
			if len(batch) >= sw.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-sw.stopCh:
			flush()
			return
		}
	}
}

// flushBatch 执行一批任务的 MySQL 写入（带重试）
func (sw *ShardWriter) flushBatch(tasks []*WriteTask) {
	if len(tasks) == 0 {
		return
	}
	ctx := context.Background()

	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := sw.exec(ctx, tasks)
		if err == nil {
			slog.Debug("shard writer batch success", "shard", sw.id, "count", len(tasks))
			return
		}
		lastErr = err
		slog.Warn("shard writer batch failed, retrying", "shard", sw.id, "attempt", attempt, "error", err)
		backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
		time.Sleep(backoff)
	}
	slog.Error("shard writer batch failed after retries", "shard", sw.id, "error", lastErr)
	// 可选: 写入死信队列或记录到文件
}

// execBatch 实际执行批量写入（使用事务）
func (sw *ShardWriter) exec(ctx context.Context, tasks []*WriteTask) error {
	tx, err := sw.mysqlRepo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, task := range tasks {
		err := tx.SaveIdempotent(ctx, task.ShortKey, task.LongURL)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (sw *ShardWriter) Stop() {
	close(sw.stopCh)
	sw.wg.Wait()
}
