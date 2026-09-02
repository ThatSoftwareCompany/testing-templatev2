package errstore

import (
	"context"
	"time"
)

type ErrorEvent struct {
	OccurredAt    time.Time
	CorrelationID string
	Method        string
	Path          string
	Endpoint      string
	StatusCode    int
	ErrorCode     string
	Message       string
}

type Filter struct {
	Endpoint string
	Limit    int
}

type Store interface {
	Persist(context.Context, ErrorEvent) error
	List(context.Context, Filter) ([]ErrorEvent, error)
}

type NoopStore struct{}

func NewNoopStore() Store {
	return NoopStore{}
}

func (NoopStore) Persist(context.Context, ErrorEvent) error {
	return nil
}

func (NoopStore) List(context.Context, Filter) ([]ErrorEvent, error) {
	return []ErrorEvent{}, nil
}
