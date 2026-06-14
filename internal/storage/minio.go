package storage

import (
	"context"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Minio implements Storage against any S3-compatible endpoint.
type Minio struct {
	c      *minio.Client
	bucket string
}

// NewMinio builds a client. It does not contact S3 until a method is called.
func NewMinio(endpoint, region, accessKey, secretKey string, useSSL bool, bucket string) (*Minio, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &Minio{c: c, bucket: bucket}, nil
}

func (m *Minio) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := m.c.PresignedPutObject(ctx, m.bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (m *Minio) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := m.c.PresignedGetObject(ctx, m.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (m *Minio) Size(ctx context.Context, key string) (int64, error) {
	info, err := m.c.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (m *Minio) Delete(ctx context.Context, key string) error {
	return m.c.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
}
