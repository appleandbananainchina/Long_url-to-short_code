package writer

import (
	"hash/fnv"
	"log/slog"
	"short-url-service/internal/repository"
	"sync"
	"time"
)

type Distributor struct {
	ringBuffer    *RingBuffer
	shardWriters  []*ShardWriter
	hashFunc      func(string) int
	wg            sync.WaitGroup
	stopCh        chan struct{}
	batchSize     int
	flushInterval time.Duration
}

func NewDistributor(numShards, ringBufferSize, batchSize int, flushInterval time.Duration, mysqlRepo *repository.MySQLRepo) *Distributor {
	d := &Distributor{
		ringBuffer:    NewRingBuffer(ringBufferSize),
		shardWriters:  make([]*ShardWriter, numShards),
		hashFunc:      func(key string) int { h := fnv.New32a(); h.Write([]byte(key)); return int(h.Sum32()) % numShards },
		stopCh:        make(chan struct{}),
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
	for i := 0; i < numShards; i++ {
		sw := NewShardWriter(i, batchSize, flushInterval, mysqlRepo)
		d.shardWriters[i] = sw
		sw.Start()
	}
	return d
}

func (d *Distributor) Start() {
	d.wg.Add(1)
	go d.dispatchLoop()
}

func (d *Distributor) dispatchLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(d.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.dispatchBatch()
		}
	}
}

func (d *Distributor) dispatchBatch() {
	tasks := d.ringBuffer.PopBatch(d.batchSize)
	if len(tasks) == 0 {
		return
	}
	// 按分片分组
	shardTasks := make([][]*WriteTask, len(d.shardWriters))
	for _, task := range tasks {
		idx := d.hashFunc(task.ShortKey)
		shardTasks[idx] = append(shardTasks[idx], task)
	}
	// 发送到各分片 channel（非阻塞，如果满了则记录丢弃）
	for idx, batch := range shardTasks {
		if len(batch) == 0 {
			continue
		}
		shard := d.shardWriters[idx]
		for _, task := range batch {
			select {
			case shard.taskCh <- task:
			default:
				slog.Warn("shard task channel full, dropping task", "shard", idx, "shortKey", task.ShortKey)
				ReleaseTask(task)
			}
		}
	}
}

// Submit 提交一个写入任务（非阻塞）
func (d *Distributor) Submit(shortKey, longURL string) bool {
	task := AcquireTask(shortKey, longURL)
	ok := d.ringBuffer.Push(task)
	if !ok {
		ReleaseTask(task)
		slog.Warn("ringbuffer full, drop write task", "shortKey", shortKey)
	}
	return ok
}

func (d *Distributor) Stop() {
	close(d.stopCh)
	d.wg.Wait()
	for _, sw := range d.shardWriters {
		sw.Stop()
	}
}
