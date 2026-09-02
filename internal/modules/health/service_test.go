package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePinger struct {
	err   error
	calls int
}

func (p *fakePinger) Ping(context.Context) error {
	p.calls++
	return p.err
}

func TestServiceReportsDisabledDatabase(t *testing.T) {
	service := NewService(nil, time.Second)
	response, ready := service.Check(context.Background())

	if !ready || response.Database != "disabled" || response.Status != "ok" {
		t.Fatalf("unexpected response: %#v, ready=%t", response, ready)
	}
}

func TestServiceReportsDatabaseUp(t *testing.T) {
	pinger := &fakePinger{}
	service := NewService(pinger, time.Second)
	response, ready := service.Check(context.Background())

	if !ready || response.Database != "up" || pinger.calls != 1 {
		t.Fatalf("unexpected response: %#v, ready=%t, calls=%d", response, ready, pinger.calls)
	}
}

func TestServiceReportsDatabaseDown(t *testing.T) {
	pinger := &fakePinger{err: errors.New("database unavailable")}
	service := NewService(pinger, time.Second)
	response, ready := service.Check(context.Background())

	if ready || response.Database != "down" || response.Status != "not_ready" {
		t.Fatalf("unexpected response: %#v, ready=%t", response, ready)
	}
}
