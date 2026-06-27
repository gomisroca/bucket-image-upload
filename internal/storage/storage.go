package storage

import (
	"context"
)

type Storage interface {
	// Persist data under given key (e.g. "123.jpg")
	Save(ctx context.Context, key string, data []byte, contentType string) error

	// Return URL to fetch data under given key (e.g. "https://example.com/123.jpg")
	ResolveURL(ctx context.Context, key string) (string, error)
}

