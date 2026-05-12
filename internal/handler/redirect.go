package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"short-url-service/pkg/statistics"
	"strings"

	"short-url-service/internal/service"
)

func RedirectHandler(shortener *service.ShortenerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 路径格式 /{shortCode}
		shortCode := strings.TrimPrefix(r.URL.Path, "/")
		if shortCode == "" {
			http.NotFound(w, r)
			return
		}

		longURL, err := shortener.GetLongURL(r.Context(), shortCode)

		if errors.Is(err, service.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "get long url failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		err = statistics.RecordAsync("redirect", shortCode)
		if err != nil {
			/**if setErr := shortener.SetIncr(r.Context(), "redirect", shortCode); setErr != nil {
				slog.WarnContext(r.Context(), "set incr failed", "error", setErr)
			}**/
			slog.ErrorContext(r.Context(), "record async failed", "error", err)
		}
		// 重定向到原始URL
		http.Redirect(w, r, longURL, http.StatusFound)
	}
}
