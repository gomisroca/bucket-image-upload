package middleware

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func RequireRateLimit(client *http.Client, limiterURL, limiterAPIKey string, failOpen bool, next http.Handler) http.Handler {
	if limiterURL == "" {
		return next
	}
	base := strings.TrimSuffix(limiterURL, "/")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Client-Key")
		if key == "" {
			key = clientIP(r)
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base+"/check?key="+url.QueryEscape(key), nil)
		if err != nil {
			log.Printf("Rate limit check: building request failed: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal error checking rate limit")
			return
		}
		if limiterAPIKey != "" {
			req.Header.Set("X-API-Key", limiterAPIKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Rate limiter unreachable: %v", err)
			if failOpen {
				next.ServeHTTP(w, r)
				return
			}
			writeJSONError(w, http.StatusServiceUnavailable, "Rate limiter unreachable")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			writeJSONError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("Rate limit check: unexpected status code: %d", resp.StatusCode)
			writeJSONError(w, http.StatusServiceUnavailable, "Rate limiter unavailable")
			return
		}

		next.ServeHTTP(w, r)
	})
}


func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Extract client IP from X-Forwarded-For header when there is X-Client-Key available
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if before, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(before)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
 
// New http.Client fit for rate-limit checks with short timeouts
func NewLimiterClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Second}
}
