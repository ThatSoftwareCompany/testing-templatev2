package errstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Persist(ctx context.Context, event ErrorEvent) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO error_events (
			occurred_at, correlation_id, method, path, endpoint,
			status_code, error_code, message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, event.OccurredAt, event.CorrelationID, event.Method, event.Path, event.Endpoint, event.StatusCode, event.ErrorCode, event.Message)
	if err != nil {
		return fmt.Errorf("persist error event: %w", err)
	}
	return nil
}

func (s *PostgresStore) List(ctx context.Context, filter Filter) ([]ErrorEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT occurred_at, correlation_id, method, path, endpoint,
		       status_code, error_code, message
		FROM error_events`
	args := []any{}
	if strings.TrimSpace(filter.Endpoint) != "" {
		query += " WHERE endpoint = $1"
		args = append(args, filter.Endpoint)
	}
	query += fmt.Sprintf(" ORDER BY occurred_at DESC LIMIT $%d", len(args)+1)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list error events: %w", err)
	}
	defer rows.Close()

	result := make([]ErrorEvent, 0)
	for rows.Next() {
		var event ErrorEvent
		if err := rows.Scan(
			&event.OccurredAt, &event.CorrelationID, &event.Method, &event.Path,
			&event.Endpoint, &event.StatusCode, &event.ErrorCode, &event.Message,
		); err != nil {
			return nil, fmt.Errorf("scan error event: %w", err)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate error events: %w", err)
	}
	return result, nil
}

func NewEvent(now time.Time, correlationID, method, path, endpoint string, statusCode int, errorCode, message string) ErrorEvent {
	return ErrorEvent{
		OccurredAt:    now,
		CorrelationID: correlationID,
		Method:        method,
		Path:          path,
		Endpoint:      endpoint,
		StatusCode:    statusCode,
		ErrorCode:     errorCode,
		Message:       message,
	}
}
