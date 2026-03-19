package snowflake

import (
	"fmt"
	"sync"
	"time"
)

const (
	nodeBits     = 10
	sequenceBits = 12
	maxNodeID    = -1 ^ (-1 << nodeBits)
	maxSequence  = -1 ^ (-1 << sequenceBits)
	timeShift    = nodeBits + sequenceBits
	nodeShift    = sequenceBits
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

type Generator struct {
	mu           sync.Mutex
	nodeID       int64
	lastUnixMS   int64
	sequenceInMS int64
}

func NewGenerator(nodeID int64) *Generator {
	if nodeID < 0 || nodeID > maxNodeID {
		nodeID = 1
	}
	return &Generator{nodeID: nodeID}
}

func (g *Generator) NextID() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UTC().UnixMilli()
	if now < g.lastUnixMS {
		return 0, fmt.Errorf("系统时间回拨，无法生成雪花 ID")
	}

	if now == g.lastUnixMS {
		g.sequenceInMS = (g.sequenceInMS + 1) & maxSequence
		if g.sequenceInMS == 0 {
			now = g.waitNextMillis(now)
		}
	} else {
		g.sequenceInMS = 0
	}

	g.lastUnixMS = now
	return ((now - epoch) << timeShift) | (g.nodeID << nodeShift) | g.sequenceInMS, nil
}

func (g *Generator) waitNextMillis(current int64) int64 {
	for {
		next := time.Now().UTC().UnixMilli()
		if next > current {
			return next
		}
		time.Sleep(time.Millisecond)
	}
}
