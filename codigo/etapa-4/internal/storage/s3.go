package storage

import (
	"bytes"
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Config struct {
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	// Endpoint é o endereço na rede interna, usado para gravar e para as URLs
	// entregues ao worker.
	Endpoint string
	// PublicEndpoint é o endereço que o cliente alcança, pelo nginx. A
	// assinatura SigV4 cobre o host, então a URL do cliente precisa ser
	// assinada com ele.
	PublicEndpoint string
}

// S3 guarda os arquivos num bucket privado do Garage. Só se lê por URL
// pré-assinada (C03, E3.3).
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
			// O Garage não precisa das somas de verificação extras que o SDK
			// envia por padrão.
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

// PresignGet gera a URL do cliente, pelo endpoint público.
func (s *S3) PresignGet(key string, ttl time.Duration) (string, error) {
	return presign(s.public, s.bucket, key, ttl)
}

// PresignInternal gera a URL do worker, que só alcança a rede de
// processamento e não tem chave do bucket (E4.2).
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
