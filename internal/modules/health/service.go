package health

import (
	"context"
	"time"
)

type Pinger interface {
	Ping(context.Context) error
}

type Service struct {
	pinger  Pinger
	timeout time.Duration
}

func NewService(pinger Pinger, timeout time.Duration) *Service {
	return &Service{pinger: pinger, timeout: timeout}
}

func (s *Service) Check(ctx context.Context) (Response, bool) {
	if s.pinger == nil {
		return Response{Status: "ok", Database: "disabled"}, true
	}

	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.pinger.Ping(checkCtx); err != nil {
		return Response{Status: "not_ready", Database: "down"}, false
	}
	return Response{Status: "ok", Database: "up"}, true
}
