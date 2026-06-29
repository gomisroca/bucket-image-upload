package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port string // e.g. "8080"
	MaxUploadBytes int64 // e.g. 10<<20 // 10 MB

	APIKey string

	StorageBackend string
	UploadDir string // used when StorageBackend is "local"

	// Used when StorageBackend == "s3". 
	S3Bucket        string
	S3Region        string
	S3Endpoint      string
	S3AccessKeyID   string
	S3SecretKey     string
	S3PublicBaseURL string
	S3PresignTTL    time.Duration
}

func Load() Config {
	cfg := Config{
		Port:           getEnv("PORT", "8080"),
		MaxUploadBytes: getEnvInt64("MAX_UPLOAD_BYTES", 10<<20), // 10 MB default
		APIKey:         getEnv("API_KEY", ""),

		StorageBackend: getEnv("STORAGE_BACKEND", "local"),
		UploadDir:      getEnv("UPLOAD_DIR", "./uploads"),

		S3Bucket:        getEnv("S3_BUCKET", ""),
		S3Region:        getEnv("S3_REGION", ""),
		S3Endpoint:      getEnv("S3_ENDPOINT", ""),
		S3AccessKeyID:   getEnv("S3_ACCESS_KEY_ID", ""),
		S3SecretKey:     getEnv("S3_SECRET_ACCESS_KEY", ""),
		S3PublicBaseURL: getEnv("S3_PUBLIC_BASE_URL", ""),
		S3PresignTTL:    time.Duration(getEnvInt64("S3_PRESIGN_TTL_SECONDS", 3600)) * time.Second,
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}