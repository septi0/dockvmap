package service

import "sync"

type WorkerActivity struct {
	mu      sync.RWMutex
	running map[string]bool
}

func NewWorkerActivity() *WorkerActivity {
	return &WorkerActivity{running: make(map[string]bool)}
}

func (a *WorkerActivity) Begin(job string) { a.set(job, true) }

func (a *WorkerActivity) End(job string) { a.set(job, false) }

func (a *WorkerActivity) Running(job string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.running[job]
}

func (a *WorkerActivity) set(job string, running bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.running[job] = running
}
