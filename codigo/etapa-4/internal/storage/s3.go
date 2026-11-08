package storage

import (
	"bytes"
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Config struct {
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	// Endpoint is the address on the internal network, used for writes and for
	// the URLs handed to the worker.
	Endpoint string
	// PublicEndpoint is the address the client reaches, through nginx. The
	// SigV4 signature covers the host, so the client's URL must be signed with
	// it.
	PublicEndpoint string
}

// S3 keeps the files in a private Garage bucket. They can only be read through
// a presigned URL (C03, E3.3).
type S3 struct {
	bucket   string
	client   *s3.Client
	public   *s3.PresignClient
	internal *s3.PresignClient
}

func NewS3(c S3Config) *S3 {
	newClient := func(endpoint string) *s3.Client {
		return s3.New(s3.Options{
			Region:       c.Region,
			BaseEndpoint: aws.String(endpoint),
			UsePathStyle: true,
			Credentials:  credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, ""),
			// Garage does not need the extra checksums the SDK sends by
			// default.
			RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
			ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		})
	}
	client := newClient(c.Endpoint)
	return &S3{
		bucket:   c.Bucket,
		client:   client,
		public:   s3.NewPresignClient(newClient(c.PublicEndpoint)),
		internal: s3.NewPresignClient(client),
	}
}

func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String("application/octet-stream"),
	})
	return err
}

// PresignGet generates the client's URL, through the public endpoint.
func (s *S3) PresignGet(key string, ttl time.Duration) (string, error) {
	return presign(s.public, s.bucket, key, ttl)
}

// PresignInternal generates the worker's URL. The worker only reaches the
// processing network and has no bucket key (E4.2).
func (s *S3) PresignInternal(key string, ttl time.Duration) (string, error) {
	return presign(s.internal, s.bucket, key, ttl)
}

func presign(c *s3.PresignClient, bucket, key string, ttl time.Duration) (string, error) {
	req, err := c.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Delete removes an object. Only the retention's key can (E8.5).
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}

// SetLifecycle is the bucket's safety net for the retention (E8.5): every
// object expires after expireDays, and an upload left incomplete is aborted
// after abortDays. It needs an owner key.
func (s *S3) SetLifecycle(ctx context.Context, expireDays, abortDays int32) error {
	_, err := s.client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(s.bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: []types.LifecycleRule{{
			ID:         aws.String("retention-safety-net"),
			Status:     types.ExpirationStatusEnabled,
			Filter:     &types.LifecycleRuleFilter{Prefix: aws.String("")},
			Expiration: &types.LifecycleExpiration{Days: aws.Int32(expireDays)},
			AbortIncompleteMultipartUpload: &types.AbortIncompleteMultipartUpload{
				DaysAfterInitiation: aws.Int32(abortDays),
			},
		}}},
	})
	return err
}
