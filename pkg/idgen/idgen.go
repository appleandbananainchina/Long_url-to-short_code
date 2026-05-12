package idgen

import (
	"sync"
	"time"
)

const (
	epoch          int64 = 1609459200000 // 2021-01-01 00:00:00 UTC 的毫秒数
	machineIDBits  uint8 = 10
	sequenceBits   uint8 = 12
	machineIDMax   int64 = -1 ^ (-1 << machineIDBits)
	sequenceMask   int64 = -1 ^ (-1 << sequenceBits)
	machineIDShift uint8 = sequenceBits
	timestampShift uint8 = sequenceBits + machineIDBits
)

type Snowflake struct {
	mu        sync.Mutex
	lastStamp int64
	machineID int64
	sequence  int64
}

func NewSnowflake(machineID int64) *Snowflake {
	if machineID < 0 || machineID > machineIDMax {
		panic("machineID out of range")
	}
	return &Snowflake{
		machineID: machineID,
		lastStamp: -1,
	}
}

func (s *Snowflake) NextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	if now < s.lastStamp {
		for now < s.lastStamp {
			// 最多等待 2 秒（避免无限循环）
			if s.lastStamp-now > 2000 {
				panic("clock moved backwards more than 2 seconds, unrecoverable")
			}
			time.Sleep(1 * time.Millisecond)
			now = time.Now().UnixMilli()
		}
	}
	if now == s.lastStamp {
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			time.Sleep(1 * time.Millisecond)
			now = time.Now().UnixMilli()
		}
	} else {
		s.sequence = 0
	}
	s.lastStamp = now

	id := (now-epoch)<<timestampShift |
		(s.machineID << machineIDShift) |
		s.sequence
	return id
}
