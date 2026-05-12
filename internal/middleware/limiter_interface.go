package middleware

import "net/http"

// Limiter 统一的限流器接口
type Limiter interface {
	// Middleware 返回 HTTP 中间件
	Middleware(next http.Handler) http.Handler
}
