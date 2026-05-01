package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"short-url-service/internal/service"
	"strings"
)

type ShortenRequest struct {
	URL       string `json:"url"`
	CustomKey string `json:"custom_key,omitempty"`
}

type ShortenResponse struct {
	ShortURL string `json:"short_url"`
}

var reservedPaths = map[string]bool{
	"shorten": true,
}
var customKeyRegex = regexp.MustCompile(`^[A-Za-z0-9_]{4,20}$`)

func validateCustomKey(key string) error {
	if key == "" {
		return fmt.Errorf("custom key is empty")
	}
	if len(key) < 4 || len(key) > 20 {
		return fmt.Errorf("custom key is invalid in length")
	}
	if !customKeyRegex.MatchString(key) {
		return fmt.Errorf("custom key is invalid in format")
	}
	lower := strings.ToLower(key)
	if reservedPaths[lower] {
		return fmt.Errorf("custom key is reserved, please choose another")
	}
	return nil
}

func ShortenHandler(shortener *service.ShortenerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ShortenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		if err2 := validateCustomKey(req.CustomKey); err2 != nil {
			http.Error(w, err2.Error(), http.StatusBadRequest)
		}
		shortCode, err := shortener.Shorten(r.Context(), req.URL, req.CustomKey)
		if err != nil {
			http.Error(w, "failed to shorten url", http.StatusInternalServerError)
			return
		}

		// 构造完整短链接，这里假设域名从配置获取或使用请求host
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		shortURL := scheme + "://" + r.Host + "/" + shortCode

		resp := ShortenResponse{ShortURL: shortURL}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
