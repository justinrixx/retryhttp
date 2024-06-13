package retryhttp

import (
	"sync/atomic"
	"time"
)

type atomicCounter struct {
	entries   []uint64
	currEntry atomic.Int32
}

func newAtomicCounter(stop <-chan bool, numEntries int, timeSpan time.Duration) *atomicCounter {
	c := atomicCounter{
		entries: make([]uint64, numEntries),
	}

	// move buckets on a timer
	t := time.NewTicker(timeSpan / time.Duration(numEntries))
	go func() {
		for {
			select {
			case <-stop:
				// the ticker cannot be gc'd until Stop is called
				t.Stop()
				return
			case <-t.C:
				// calculate next index
				next := (c.currEntry.Load() + 1) % int32(numEntries)

				// clear the next bucket
				atomic.StoreUint64(&c.entries[next], 0)

				// update current index to next
				c.currEntry.Store(next)
			}
		}
	}()

	return &c
}

func (c *atomicCounter) increment() {
	atomic.AddUint64(&c.entries[c.currEntry.Load()], 1)
}

func (c *atomicCounter) read() uint64 {
	var sum uint64
	for _, entry := range c.entries {
		sum += entry
	}

	return sum
}
