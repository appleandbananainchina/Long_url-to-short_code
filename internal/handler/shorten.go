package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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

func validateLongURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("url is empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported protocol: %s, only http/https allowed", parsed.Scheme)
	}
	// 可选：限制 host 不能为空
	if parsed.Host == "" {
		return fmt.Errorf("missing host")
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

		if err := validateLongURL(req.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.CustomKey != "" {
			if err2 := validateCustomKey(req.CustomKey); err2 != nil {
				http.Error(w, err2.Error(), http.StatusBadRequest)
				return
			}
		}
		shortCode, err := shortener.Shorten(r.Context(), req.URL, req.CustomKey)
		if err != nil {
			if errors.Is(err, service.ErrConflict) {
				http.Error(w, "custom key already exists", http.StatusConflict)
				return
			}
			slog.ErrorContext(r.Context(), "shorten failed", "error", err)
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
