// Package uploads provides file upload services backed by Cloudflare R2
// (S3-compatible object storage). Used for payment screenshots and other
// customer-uploaded files.
package uploads

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Service defines the interface for file upload operations.
type Service interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

type service struct {
	client        *s3.Client
	bucket        string
	publicURLBase string
}

// NewService creates an R2-backed upload service using the given credentials.
func NewService(accountID, accessKeyID, secretKey, bucket, publicURLBase string) (Service, error) {
	endpointURL := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, ""),
		),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("r2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
		o.UsePathStyle = true
	})

	return &service{client: client, bucket: bucket, publicURLBase: publicURLBase}, nil
}

func (s *service) Put(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("r2 put: %w", err)
	}
	return s.URL(key), nil
}

func (s *service) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *service) URL(key string) string {
	if s.publicURLBase != "" {
		return s.publicURLBase + "/" + key
	}
	return fmt.Sprintf("https://pub-%s.r2.dev/%s", s.bucket, key)
}

// GenerateKey creates a unique object key with a prefix and nanosecond timestamp.
func GenerateKey(prefix, filename string) string {
	return fmt.Sprintf("%s/%d_%s", prefix, time.Now().UnixNano(), filename)
}
