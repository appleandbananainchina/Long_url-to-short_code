package handler

import (
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
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if longURL == "" {
			http.NotFound(w, r)
			return
		}
		err = statistics.RecordAsync("redirect", shortCode)
		if err != nil {
			shortener.SetIncr(r.Context(), "redirect", shortCode)
		}
		// 重定向到原始URL
		http.Redirect(w, r, longURL, http.StatusFound)
	}
}
