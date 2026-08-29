package service

type WorkerTrigger struct {
	channels map[string]chan struct{}
}

func NewWorkerTrigger(jobs ...string) *WorkerTrigger {
	channels := make(map[string]chan struct{}, len(jobs))

	for _, job := range jobs {
		channels[job] = make(chan struct{}, 1)
	}

	return &WorkerTrigger{channels: channels}
}

func (t *WorkerTrigger) Channel(job string) <-chan struct{} {
	return t.channels[job]
}

func (t *WorkerTrigger) Trigger(job string) bool {
	ch, ok := t.channels[job]

	if !ok {
		return false
	}

	// non-blocking: an already-pending trigger is enough, extra ones coalesce
	select {
	case ch <- struct{}{}:
	default:
	}

	return true
}
