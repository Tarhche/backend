package minio

import (
	"context"
	"io"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func NewMinIOClient() (*minio.Client, error) {
	endpoint := "play.min.io"
	accessKeyID := "ppbmqazyCzaip1cZO46g"
	secretAccessKey := "hKwXWnaaEKHn7okncrWDVZ4OxKImHyfJJjPftxpT"
	useSSL := true

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})

	if err != nil {
		return nil, err
	}

	return minioClient, nil
}

func CreateBucket(ctx context.Context, client *minio.Client, bucketName string) error {

	if exists, err := client.BucketExists(ctx, bucketName); err != nil {
		return err
	} else if exists {
		return nil
	}

	return client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
}

func CreateObject(ctx context.Context, client *minio.Client, bucketName string, objectName string, reader io.Reader, objectSize int64, options minio.PutObjectOptions) error {
	info, err := client.PutObject(ctx, bucketName, objectName, reader, objectSize, options)
	if err != nil {
		return err
	}

	log.Println(info)
	return nil
}

func DeleteObject(ctx context.Context, client *minio.Client, bucketName string, objectName string) error {
	return client.RemoveObject(ctx, bucketName, objectName, minio.RemoveObjectOptions{})
}

func ReadObject(ctx context.Context, client *minio.Client, bucketName string, objectName string) (*minio.Object, error) {
	obj, err := client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	return obj, nil
}
