package minio

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Options struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	UseSSL     bool
	BucketName string
}

type MinIO struct {
	client     *minio.Client
	bucketName string
}

func New(opt Options) (*MinIO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	minioClient, err := minio.New(opt.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opt.AccessKey, opt.SecretKey, ""),
		Secure: opt.UseSSL,
	})

	if err != nil {
		return nil, err
	}

	if err := createBucket(ctx, minioClient, opt.BucketName); err != nil {
		return nil, err
	}

	return &MinIO{
		client:     minioClient,
		bucketName: opt.BucketName,
	}, nil
}

func (storage *MinIO) Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error {
	_, err := storage.client.PutObject(ctx, storage.bucketName, objectName, reader, objectSize, minio.PutObjectOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (storage *MinIO) Delete(ctx context.Context, objectName string) error {
	return storage.client.RemoveObject(ctx, storage.bucketName, objectName, minio.RemoveObjectOptions{})
}

func (storage *MinIO) Read(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj, err := storage.client.GetObject(ctx, storage.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func createBucket(ctx context.Context, client *minio.Client, bucketName string) error {
	if exists, err := client.BucketExists(ctx, bucketName); err != nil {
		return err
	} else if exists {
		return nil
	}

	return client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
}
