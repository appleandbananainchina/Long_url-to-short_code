package middleware

import (
	"log/slog"
	"net/http"
)

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// 尝试从 context 中获取 trace_id
				traceID := ""
				if r.Context() != nil {
					if tid, ok := r.Context().Value("trace_id").(string); ok {
						traceID = tid
					}
				}
				// 如果 context 中没有，则从请求头获取
				if traceID == "" {
					traceID = r.Header.Get("X-Trace-ID")
				}

				// 记录 panic 信息，附带请求关键字段
				slog.Error("panic recovered",
					"trace_id", traceID,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
					"error", err,
				)

				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
