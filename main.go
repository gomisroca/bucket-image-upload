package main

import (
	"bucket-image-upload/internal/config"
	"bucket-image-upload/internal/storage"
	"log"
)

func main() {
	cfg := config.Load()

	store, err := buildStorage(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
}

func buildStorage(cfg config.Config) (storage.Storage, error) {
	switch cfg.StorageBackend {
	case "s3":
		// return storage.NewS3Storage()
	default:
		// return storage.NewLocalStorage(cfg.UploadDir, "/uploads")
	}
}