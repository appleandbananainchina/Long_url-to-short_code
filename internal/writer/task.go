package writer

import (
	"sync"
)

// WriteTask 代表一个待写入的短链映射
type WriteTask struct {
	ShortKey string
	LongURL  string
}

var taskPool = sync.Pool{
	New: func() interface{} { return &WriteTask{} },
}

// AcquireTask 从对象池获取任务
func AcquireTask(shortKey, longURL string) *WriteTask {
	task := taskPool.Get().(*WriteTask)
	task.ShortKey = shortKey
	task.LongURL = longURL
	return task
}

// ReleaseTask 将任务归还对象池
func ReleaseTask(task *WriteTask) {
	task.ShortKey = ""
	task.LongURL = ""
	taskPool.Put(task)
}
