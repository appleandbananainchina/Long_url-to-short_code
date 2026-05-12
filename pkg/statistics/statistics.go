package statistics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"short-url-service/pkg/cache"
	"short-url-service/pkg/metrics"
	"sync"
	"time"
)

type Task struct {
	application string
	key         string
}

type StatisticsWorker struct {
	redisCli  *cache.RedisClient
	taskQueue chan Task
	wg        sync.WaitGroup
	stopCh    chan struct{}
}

var (
	worker     *StatisticsWorker
	workerOnce sync.Once
)

func InitStatisticsWorker(poolSize, queueSize int, redisCli *cache.RedisClient) {
	workerOnce.Do(func() {
		worker = &StatisticsWorker{
			taskQueue: make(chan Task, queueSize),
			stopCh:    make(chan struct{}),
			redisCli:  redisCli,
		}
		worker.start(poolSize)
	})
}

func (worker *StatisticsWorker) start(poolSize int) {
	for i := 0; i < poolSize; i++ {
		worker.wg.Add(1)
		go worker.worker(i)
	}
}

func (worker *StatisticsWorker) worker(id int) {
	defer worker.wg.Done()
	ctx := context.Background()
	for {
		select {
		case task, ok := <-worker.taskQueue:
			if !ok {
				return
			}
			// 执行统计：Redis INCR
			if err := worker.redisCli.Incr(ctx, fmt.Sprintf("stats:%s:%s", task.application, task.key)).Err(); err != nil {
				slog.ErrorContext(context.Background(), "statistics record failed",
					"worker_id", id,
					"application", task.application,
					"key", task.key,
					"error", err,
				)
			}
		case <-worker.stopCh:
			return
		}
	}
}

func RecordAsync(application, key string) error {
	if worker == nil {
		return errors.New("worker is nil")
	}
	task := Task{application, key}
	select {
	case worker.taskQueue <- task:
		metrics.StatisticsQueueLength.Set(float64(len(worker.taskQueue)))
		return nil
	default:
		metrics.StatisticsQueueDropped.Inc()
		return errors.New("task queue is full")
	}
}

func StopWorker(timeout time.Duration) {
	close(worker.taskQueue)
	done := make(chan struct{})
	go func() {
		worker.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("stop worker success")
	case <-time.After(timeout):
		slog.Warn("statistics worker stop timeout")
		close(worker.stopCh)
		worker.wg.Wait()
	}
}

func GetQueueLength() int {
	if worker == nil {
		return 0
	}
	return len(worker.taskQueue)
}
