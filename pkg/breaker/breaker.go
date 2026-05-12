package breaker

import (
	"context"
	"short-url-service/pkg/metrics"
	"time"

	"github.com/sony/gobreaker/v2"
)

type Breaker[T any] struct {
	cb *gobreaker.CircuitBreaker[T]
}

type Config struct {
	Name                string
	MaxRequests         uint32
	Timeout             time.Duration
	ConsecutiveFailures uint32
}

func New[T any](config Config) *Breaker[T] {
	settings := gobreaker.Settings{
		Name:        config.Name,
		MaxRequests: config.MaxRequests,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > config.ConsecutiveFailures
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			stateValue := 0.0
			switch to {
			case gobreaker.StateClosed:
				stateValue = 0
			case gobreaker.StateOpen:
				stateValue = 1
			case gobreaker.StateHalfOpen:
				stateValue = 2
			}
			metrics.CircuitBreakerState.WithLabelValues(name).Set(stateValue)
		},
	}
	return &Breaker[T]{
		cb: gobreaker.NewCircuitBreaker[T](settings),
	}
}

func (b *Breaker[T]) Do(ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		val T
		err error
	}
	resChan := make(chan result, 1)
	go func() {
		val, err := b.cb.Execute(fn)
		resChan <- result{val, err}
	}()
	select {
	case res := <-resChan:
		return res.val, res.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func (b *Breaker[T]) IsOpen() bool {
	return b.cb.State() == gobreaker.StateOpen
}
