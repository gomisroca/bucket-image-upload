package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	dir string
	urlPrefix string
}

func NewLocalStorage(dir, urlPrefix string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("Creating upload dir: %w", err)
	}
	return &LocalStorage{dir: dir, urlPrefix: urlPrefix}, nil
}

func (l *LocalStorage) Save(_ context.Context, key string, data []byte, _ string) error {
	path := filepath.Join(l.dir, key)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("Writing file %s: %w", key, err)
	}
	return nil
}

func (l *LocalStorage) ResolveURL(_ context.Context, key string) (string, error) {
	return filepath.Join(l.urlPrefix, key), nil
}