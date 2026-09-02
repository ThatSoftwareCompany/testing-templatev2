package errstore

import (
	"context"
	"testing"
)

func TestNoopStoreDoesNotPersistOrListEvents(t *testing.T) {
	store := NewNoopStore()

	if err := store.Persist(context.Background(), ErrorEvent{Message: "must not be stored"}); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	items, err := store.List(context.Background(), Filter{Endpoint: "/internal", Limit: 1})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List() returned %d items, want zero", len(items))
	}
}
