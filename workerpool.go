package breeze

import (
	"context"
	"runtime"
	"sync"
)

// WorkerPool dispatches tasks to a fixed pool of goroutines.
//
// Performance decisions:
//   - Channel buffer is set to 16× the worker count (not 1×) so that a burst
//     of requests doesn't block gnet's event-loop goroutine while workers
//     catch up. The event loop is single-threaded per reactor; blocking it
//     stalls ALL connections on that reactor.
//   - defaultWorkerMultiplier × NumCPU is the sweet spot for I/O-bound
//     handlers. CPU-bound handlers should use NumCPU directly.
const defaultChannelMultiplier = 16

type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
	count int
}

// NewWorkerPool creates a pool with `concurrency` goroutines and a task queue
// of concurrency × defaultChannelMultiplier to absorb request bursts without
// blocking gnet's event loops.
func NewWorkerPool(concurrency int) *WorkerPool {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	bufSize := concurrency * defaultChannelMultiplier
	p := &WorkerPool{
		tasks: make(chan func(), bufSize),
		count: concurrency,
	}
	for i := 0; i < concurrency; i++ {
		go func() {
			for task := range p.tasks {
				task()
			}
		}()
	}
	return p
}

// Submit enqueues a task. It never blocks as long as the queue has capacity;
// if the queue is full it falls back to spawning a goroutine so the event
// loop is never stalled.
func (p *WorkerPool) Submit(f func()) {
	p.wg.Add(1)
	task := func() {
		defer p.wg.Done()
		f()
	}
	select {
	case p.tasks <- task:
		// queued normally
	default:
		// Queue full: spawn a goroutine rather than blocking the event loop.
		go task()
	}
}

// Shutdown waits for all in-flight tasks to complete or for ctx to expire.
func (p *WorkerPool) Shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	close(p.tasks)
}
