package minio

import (
	"context"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Options struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	UseSSL     bool
	BucketName string

	// PartSize is how much of an object is held in memory at a time while it
	// is stored in parts, in bytes. Zero is minio's own choice, which for an
	// object of unknown size (stored with a size of -1) is parts of about
	// 512 MiB: fine for an upload, and far too much for a service streaming
	// an archive with a memory limit. At least 5 MiB, which S3 requires.
	PartSize uint64
}

type MinIO struct {
	client     *minio.Client
	bucketName string
	partSize   uint64
	tracer     oteltrace.Tracer
}

var _ file.Storage = &MinIO{}

func New(opt Options) (*MinIO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		partSize:   opt.PartSize,
		tracer:     otel.Tracer("minio"),
	}, nil
}

func (storage *MinIO) Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error {
	ctx, span := storage.tracer.Start(ctx, "minio.store",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(attribute.String("object", objectName), attribute.Int64("size", objectSize)),
	)
	defer span.End()

	_, err := storage.client.PutObject(ctx, storage.bucketName, objectName, reader, objectSize, minio.PutObjectOptions{
		PartSize: storage.partSize,
	})

	return trace.RecordError(span, err)
}

func (storage *MinIO) Delete(ctx context.Context, objectName string) error {
	ctx, span := storage.tracer.Start(ctx, "minio.delete",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(attribute.String("object", objectName)),
	)
	defer span.End()

	return trace.RecordError(span, storage.client.RemoveObject(ctx, storage.bucketName, objectName, minio.RemoveObjectOptions{}))
}

func (storage *MinIO) Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error) {
	ctx, span := storage.tracer.Start(ctx, "minio.read",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(attribute.String("object", objectName)),
	)
	defer span.End()

	obj, err := storage.client.GetObject(ctx, storage.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, trace.RecordError(span, err)
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
