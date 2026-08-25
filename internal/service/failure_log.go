package service

import (
	"sync"
	"time"
)

type FailureSource string

const (
	FailureSourceWebhook           FailureSource = "webhook"
	FailureSourceEmail             FailureSource = "email"
	FailureSourceRefresh           FailureSource = "refresh"
	FailureSourceDiscoveryRefresh  FailureSource = "discovery_refresh"
	FailureSourceEventRegistration FailureSource = "event_registration"
)

type Failure struct {
	Source     FailureSource
	Detail     string
	Error      string
	OccurredAt time.Time
}

const maxFailureLogEntries = 20

type FailureLog struct {
	mu      sync.Mutex
	entries []Failure
}

func NewFailureLog() *FailureLog {
	return &FailureLog{}
}

func (l *FailureLog) Record(source FailureSource, detail string, err error) {
	if err == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, Failure{
		Source:     source,
		Detail:     detail,
		Error:      err.Error(),
		OccurredAt: time.Now().UTC(),
	})

	if len(l.entries) > maxFailureLogEntries {
		l.entries = l.entries[len(l.entries)-maxFailureLogEntries:]
	}
}

func (l *FailureLog) Recent() []Failure {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]Failure, len(l.entries))

	for i, entry := range l.entries {
		out[len(l.entries)-1-i] = entry
	}

	return out
}
