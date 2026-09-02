package errors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/errstore"
)

type fakeStore struct {
	events []errstore.ErrorEvent
	err    error
	filter errstore.Filter
}

func (s *fakeStore) Persist(context.Context, errstore.ErrorEvent) error { return nil }

func (s *fakeStore) List(_ context.Context, filter errstore.Filter) ([]errstore.ErrorEvent, error) {
	s.filter = filter
	if s.err != nil {
		return nil, s.err
	}
	return s.events, nil
}

func TestServiceListMapsSafeErrorEventsAndFilters(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{events: []errstore.ErrorEvent{{
		OccurredAt:    occurredAt,
		CorrelationID: "correlation-1",
		Method:        "GET",
		Path:          "/api/v1/health",
		Endpoint:      "/api/v1/health",
		StatusCode:    503,
		ErrorCode:     "service_unavailable",
		Message:       "service unavailable",
	}}}

	response, err := NewService(store).List(context.Background(), ListRequest{Endpoint: "/api/v1/health", Limit: 25})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if store.filter.Endpoint != "/api/v1/health" || store.filter.Limit != 25 {
		t.Fatalf("unexpected filter: %#v", store.filter)
	}
	if len(response.Items) != 1 || response.Items[0].CorrelationID != "correlation-1" || !response.Items[0].OccurredAt.Equal(occurredAt) {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServiceListPropagatesStoreErrors(t *testing.T) {
	wantErr := errors.New("store unavailable")
	_, err := NewService(&fakeStore{err: wantErr}).List(context.Background(), ListRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("List() error = %v, want %v", err, wantErr)
	}
}
