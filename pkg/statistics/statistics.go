package statistics

import (
	"context"
	"errors"
	"log"
	"short-url-service/pkg/cache"
	"sync"
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
			if err := worker.redisCli.Incr(ctx, "stats:"+task.application+task.key).Err(); err != nil {
				log.Printf("worker[%d] record %s failed for %s: %v", id, task.application, task.key, err)
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
		return nil
	default:
		// 队列满，丢弃任务并记录日志
		return errors.New("task queue is full")
	}
}

func StopWorker() {
	if worker == nil {
		return
	}
	close(worker.stopCh)
	close(worker.taskQueue)
	worker.wg.Wait()
	log.Println("stop worker success")
}

func GetQueueLength() int {
	if worker == nil {
		return 0
	}
	return len(worker.taskQueue)
}
