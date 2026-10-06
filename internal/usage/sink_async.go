package usage

import (
	"sync"
	"sync/atomic"
	"time"
)

// AsyncSink runs a sink command off the caller's goroutine. A long-running
// server records a skill load on its request path; a hung sink must not add its
// timeout to every load, so lines wait in a bounded queue and are dropped (and
// counted) when the queue is full.
type AsyncSink struct {
	command string
	queue   chan []byte
	onError func(error)
	dropped atomic.Int64
	done    chan struct{}
	once    sync.Once
}

// NewAsyncSink starts the worker. onError receives every failed run and may be nil.
func NewAsyncSink(command string, queueSize int, onError func(error)) *AsyncSink {
	if queueSize < 1 {
		queueSize = 1
	}
	s := &AsyncSink{command: command, queue: make(chan []byte, queueSize), onError: onError, done: make(chan struct{})}
	go s.run()
	return s
}

func (s *AsyncSink) run() {
	defer close(s.done)
	for line := range s.queue {
		if err := runSink(s.command, line); err != nil && s.onError != nil {
			s.onError(err)
		}
	}
}

// Send queues a line; it reports false when the queue is full and the line was dropped.
func (s *AsyncSink) Send(line []byte) (queued bool) {
	defer func() {
		if recover() != nil { // sent after Close
			s.dropped.Add(1)
			queued = false
		}
	}()
	select {
	case s.queue <- line:
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

// Dropped counts the lines that were not delivered because the queue was full.
func (s *AsyncSink) Dropped() int64 { return s.dropped.Load() }

// Close stops accepting lines and waits up to wait for the queued ones to be
// delivered; a zero wait does not wait.
func (s *AsyncSink) Close(wait time.Duration) {
	s.once.Do(func() { close(s.queue) })
	if wait <= 0 {
		return
	}
	select {
	case <-s.done:
	case <-time.After(wait):
	}
}
