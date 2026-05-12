package middleware

import "net/http"

type NoopLimiter struct{}

func (NoopLimiter) Middleware(next http.Handler) http.Handler {
	return next
}
