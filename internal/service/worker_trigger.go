package service

import "sync"

type WorkerTrigger struct {
	mu       sync.Mutex
	channels map[string]chan struct{}
}

func NewWorkerTrigger() *WorkerTrigger {
	return &WorkerTrigger{channels: make(map[string]chan struct{})}
}

func (t *WorkerTrigger) Register(job string) <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch, ok := t.channels[job]

	if !ok {
		ch = make(chan struct{}, 1)
		t.channels[job] = ch
	}

	return ch
}

func (t *WorkerTrigger) Trigger(job string) bool {
	t.mu.Lock()
	ch, ok := t.channels[job]
	t.mu.Unlock()

	if !ok {
		return false
	}

	select {
	case ch <- struct{}{}:
	default:
	}

	return true
}
