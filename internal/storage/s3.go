package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	client *s3.Client
	presign *s3.PresignClient
	bucket string
	publicBaseURL string // optional, skips presign
	presignTTL time.Duration // used if publicBaseURL is empty
}

// For AWS S3: leave Endpoint empty, set Region to bucket's region (e.g. "us-east-1")
// For Cloudflare R2: set Endpoint to "https://<account_id>.r2.cloudflarestorage.com" and Region to "auto"
// Generate AccessKeyID/SecretAccessKey from the R2 dashboard ("Manage R2 API Tokens")
type S3Config struct {
	Bucket string
	Region string
	Endpoint string // optional, custom endpoint
	AccessKeyID string
	SecretAccessKey string
	PublicBaseURL string // optional, public bucket url
	PresignTTL time.Duration // used if PublicBaseURL is empty
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("S3 Storage requires bucket, access key id and secret access key")
	}

	region := cfg.Region
	if region == "" {
		region = "auto" // R2 requires region to be set
	}

	awsCfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		}
	})

	ttl := cfg.PresignTTL
	if ttl <= 0 {
		ttl = time.Hour
	}

	return &S3Storage{
		client: client,
		presign: s3.NewPresignClient(client),
		bucket: cfg.Bucket,
		publicBaseURL: strings.TrimSuffix(cfg.PublicBaseURL, "/"),
		presignTTL: ttl,
	}, nil
}

func (s *S3Storage) Save(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key: aws.String(key),
		Body: bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("Uploading %q to S3-compatible storage: %w", key, err)
	}
	return nil
}

func (s *S3Storage) ResolveURL(ctx context.Context, key string) (string, error) {
	// If publicBaseURL is set, directly return the public URL
	if s.publicBaseURL != "" {
		return fmt.Sprintf("%s/%s", s.publicBaseURL, key), nil
	}

	// Otherwise, presign the URL
	presigned, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key: aws.String(key),
	}, s3.WithPresignExpires(s.presignTTL))
	if err != nil {
		return "", fmt.Errorf("Presigning %q: %w", key, err)
	}
	return presigned.URL, nil
}