package errors

import (
	"context"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/errstore"
)

type Service struct {
	store errstore.Store
}

func NewService(store errstore.Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, request ListRequest) (ListResponse, error) {
	items, err := s.store.List(ctx, errstore.Filter{Endpoint: request.Endpoint, Limit: request.Limit})
	if err != nil {
		return ListResponse{}, err
	}
	response := ListResponse{Items: make([]ErrorEvent, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, ErrorEvent{
			OccurredAt:    item.OccurredAt,
			CorrelationID: item.CorrelationID,
			Method:        item.Method,
			Path:          item.Path,
			Endpoint:      item.Endpoint,
			StatusCode:    item.StatusCode,
			ErrorCode:     item.ErrorCode,
			Message:       item.Message,
		})
	}
	return response, nil
}
