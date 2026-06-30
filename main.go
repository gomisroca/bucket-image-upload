package main

import (
	"bucket-image-upload/internal/config"
	"bucket-image-upload/internal/handlers"
	"bucket-image-upload/internal/middleware"
	"bucket-image-upload/internal/storage"
	"log"
	"net/http"
	"time"
)

func main() {
	cfg := config.Load()

	store, err := buildStorage(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	uploadHandler := handlers.NewUploadHandler(store, cfg.MaxUploadBytes)
	filesHandler := handlers.NewFilesHandler(store)
	
	if cfg.APIKey == "" {
		log.Println("WARNING: API_KEY is not set - POST '/upload' is open to anyone who can reach this service. Set API_KEY before deploying publicly.")
	}
	if cfg.RateLimiterURL == "" {
		log.Println("NOTE: RATELIMITER_URL is not set - '/upload' has no rate limiting of its own; it relies entirely on whatever calls it to enforce limits upstream.")
	}

	limiterClient := middleware.NewLimiterClient()
	protectedUpload := middleware.RequireAPIKey(cfg.APIKey, middleware.RequireRateLimit(limiterClient, cfg.RateLimiterURL, cfg.RateLimiterAPIKey, cfg.RateLimiterFailOpen, uploadHandler))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)
	mux.Handle("POST /upload", protectedUpload)
	mux.Handle("GET /files/{key}", filesHandler)
	// Static file serving, local storage only
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadDir))))
  	// Test page
	mux.Handle("GET /", http.FileServer(http.Dir("./web")))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      withCORS(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("bucket-upload-service listening on :%s (storage backend: %s)", cfg.Port, cfg.StorageBackend)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// Pick Storage implementation based on config
func buildStorage(cfg config.Config) (storage.Storage, error) {
	switch cfg.StorageBackend {
	case "s3":
		return storage.NewS3Storage(storage.S3Config{
			Bucket:          cfg.S3Bucket,
			Region:          cfg.S3Region,
			Endpoint:        cfg.S3Endpoint,
			AccessKeyID:     cfg.S3AccessKeyID,
			SecretAccessKey: cfg.S3SecretKey,
			PublicBaseURL:   cfg.S3PublicBaseURL,
			PresignTTL:      cfg.S3PresignTTL,
		})
	default:
		return storage.NewLocalStorage(cfg.UploadDir, "/uploads")
	}
}


// Allow specific origins
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}