package writer

import (
	"sync/atomic"
	"unsafe"
)

type RingBuffer struct {
	buffer []unsafe.Pointer // 实际存储 *WriteTask
	size   uint32
	head   uint32 // 读索引
	tail   uint32 // 写索引
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buffer: make([]unsafe.Pointer, size),
		size:   uint32(size),
	}
}

// Push 非阻塞入队，成功返回 true，满则 false
func (rb *RingBuffer) Push(task *WriteTask) bool {
	for {
		tail := atomic.LoadUint32(&rb.tail)
		head := atomic.LoadUint32(&rb.head)
		if tail-head >= rb.size {
			return false // 缓冲区满
		}
		newTail := tail + 1
		if atomic.CompareAndSwapUint32(&rb.tail, tail, newTail) {
			idx := tail % rb.size
			atomic.StorePointer(&rb.buffer[idx], unsafe.Pointer(task))
			return true
		}
		// CAS 失败，重试
	}
}

// PopBatch 批量取出最多 batchSize 个任务
func (rb *RingBuffer) PopBatch(batchSize int) []*WriteTask {
	var tasks []*WriteTask
	for i := 0; i < batchSize; i++ {
		head := atomic.LoadUint32(&rb.head)
		tail := atomic.LoadUint32(&rb.tail)
		if head >= tail {
			break
		}
		idx := head % rb.size
		ptr := atomic.SwapPointer(&rb.buffer[idx], nil)
		if ptr != nil {
			task := (*WriteTask)(ptr)
			tasks = append(tasks, task)
			atomic.AddUint32(&rb.head, 1)
		}
	}
	return tasks
}

// Len 返回当前缓冲区任务数（近似）
func (rb *RingBuffer) Len() uint32 {
	tail := atomic.LoadUint32(&rb.tail)
	head := atomic.LoadUint32(&rb.head)
	if tail >= head {
		return tail - head
	}
	return 0
}
