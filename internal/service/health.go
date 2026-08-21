package service

import "context"

type healthStore interface {
	Ping(ctx context.Context) error
}

type Health struct {
	store healthStore
}

func NewHealth(store healthStore) *Health {
	return &Health{store: store}
}

func (h *Health) Ping(ctx context.Context) error {
	return h.store.Ping(ctx)
}
