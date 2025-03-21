package store

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/mawngo/go-try/v2"
	"os"
	"path"
	"sin/internal/utils"
	"strings"
	"time"
)

const (
	MB = 1024 * 1024

	defaultPartSizeMB  = 50
	defaultThresholdMB = 110
)

var _ Adapter = (*s3Adapter)(nil)

// s3Adapter is not safe for concurrent use.
type s3Adapter struct {
	AdapterConfig
	Multipart    s3MultipartConfig `json:"multipart"`
	Bucket       string            `json:"bucket"`
	Endpoint     string            `json:"endpoint"`
	AccessKeyID  string            `json:"accessKeyID"`
	AccessSecret string            `json:"accessSecret"`
	Region       string            `json:"region"`
	BasePath     string            `json:"basePath"`

	client          *s3.Client
	recentlyDeleted map[string]struct{}
}

type s3MultipartConfig struct {
	ThresholdMB     int  `json:"thresholdMB"`
	PartSizeMB      int  `json:"partSizeMB"`
	Concurrency     int  `json:"concurrency"`
	DisableChecksum bool `json:"disableChecksum"`
}

func newS3Adapter(conf map[string]any) (Adapter, error) {
	adapter := s3Adapter{}
	if err := utils.MapToStruct(conf, &adapter); err != nil {
		return nil, err
	}
	if adapter.Bucket == "" {
		return nil, errors.New("missing bucket config for s3 adapter " + adapter.Name)
	}
	if adapter.Endpoint == "" {
		return nil, errors.New("missing endpoint config for s3 adapter " + adapter.Name)
	}
	if adapter.AccessKeyID == "" {
		return nil, errors.New("missing accessKeyID config for s3 adapter " + adapter.Name)
	}
	if adapter.AccessSecret == "" {
		return nil, errors.New("missing accessSecret config for s3 adapter " + adapter.Name)
	}
	if adapter.Region == "" {
		adapter.Region = "auto"
	}
	if adapter.Multipart.PartSizeMB < 5 || adapter.Multipart.PartSizeMB > 4*1024 {
		adapter.Multipart.PartSizeMB = defaultPartSizeMB
	}
	if adapter.Multipart.ThresholdMB < 20 || adapter.Multipart.ThresholdMB > 4*1024 {
		adapter.Multipart.ThresholdMB = defaultThresholdMB
	}
	return &adapter, nil
}

func (f *s3Adapter) Save(ctx context.Context, source string, pathElem string, pathElems ...string) error {
	p := f.joinPath(pathElem, pathElems...)
	checksum, err := utils.FileSHA256Checksum(source)
	if err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return err
	}
	if fi.Size() < int64(f.Multipart.ThresholdMB*MB) {
		return f.upload(ctx, p, file, checksum)
	}
	return f.uploadMultipart(ctx, p, file, checksum)
}

func (f *s3Adapter) uploadMultipart(ctx context.Context, p string, file *os.File, checksum []byte) error {
	s3Client, err := f.getClient(ctx)
	if err != nil {
		return err
	}
	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = int64(min(f.Multipart.PartSizeMB, 10) * MB)
		u.Concurrency = f.Multipart.Concurrency
	})

	input := &s3.PutObjectInput{
		Bucket: aws.String(f.Bucket),
		Key:    aws.String(p),
		Body:   file,
	}
	if !f.Multipart.DisableChecksum {
		input.ChecksumAlgorithm = types.ChecksumAlgorithmSha256
		c := base64.StdEncoding.EncodeToString(checksum)
		input.ChecksumSHA256 = &c
	}

	// TODO: should we retry this?
	_, err = uploader.Upload(ctx, input)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" {
			return errors.New("object too large")
		}
		return err
	}

	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx,
		&s3.HeadObjectInput{Bucket: aws.String(f.Bucket), Key: aws.String(p)},
		5*time.Minute)
	if err != nil {
		return fmt.Errorf("error waiting for object %s: %w", p, err)
	}
	return f.uploadChecksum(ctx, p, hex.EncodeToString(checksum))
}

func (f *s3Adapter) upload(ctx context.Context, p string, file *os.File, checksum []byte) error {
	s3Client, err := f.getClient(ctx)
	if err != nil {
		return err
	}

	c := base64.StdEncoding.EncodeToString(checksum)
	_, err = try.GetCtx(ctx, func() (*s3.PutObjectOutput, error) {
		return s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:            aws.String(f.Bucket),
			Key:               aws.String(p),
			Body:              file,
			ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
			ChecksumSHA256:    &c,
		})
	}, try.WithFixedBackoff(10*time.Second))
	if err != nil {
		return err
	}
	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx,
		&s3.HeadObjectInput{Bucket: aws.String(f.Bucket), Key: aws.String(p)},
		5*time.Minute)
	if err != nil {
		return fmt.Errorf("error waiting for object %s: %w", p, err)
	}
	return f.uploadChecksum(ctx, p, hex.EncodeToString(checksum))
}

func (f *s3Adapter) uploadChecksum(ctx context.Context, p string, checksum string) error {
	s3Client, err := f.getClient(ctx)
	if err != nil {
		return err
	}

	_, err = try.GetCtx(ctx, func() (*s3.PutObjectOutput, error) {
		return s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(f.Bucket),
			Key:    aws.String(p + utils.ChecksumExt),
			Body:   strings.NewReader(checksum),
		})
	}, try.WithFixedBackoff(10*time.Second))
	if err != nil {
		return err
	}
	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx,
		&s3.HeadObjectInput{Bucket: aws.String(f.Bucket), Key: aws.String(p)},
		5*time.Minute)
	if err != nil {
		return fmt.Errorf("error waiting for object checksum %s: %w", p, err)
	}
	return nil
}

func (f *s3Adapter) Del(ctx context.Context, pathElem string, pathElems ...string) error {
	p := f.joinPath(pathElem, pathElems...)
	s3Client, err := f.getClient(ctx)
	if err != nil {
		return err
	}

	err = try.DoCtx(ctx, func() error {
		_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(f.Bucket),
			Key:    aws.String(p),
		})
		return err
	}, try.WithFixedBackoff(10*time.Second))

	if err != nil {
		return err
	}

	return try.DoCtx(ctx, func() error {
		_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(f.Bucket),
			Key:    aws.String(p + utils.ChecksumExt),
		})
		return err
	}, try.WithFixedBackoff(10*time.Second))
}

func (f *s3Adapter) ListFileNames(ctx context.Context, pathElems ...string) ([]string, error) {
	p := f.joinPath("", pathElems...)
	s3Client, err := f.getClient(ctx)
	if err != nil {
		return nil, err
	}

	params := s3.ListObjectsV2Input{
		Bucket: aws.String(f.Bucket),
	}
	if p != "" {
		params.Prefix = aws.String(p + "/")
	}

	// Create the Paginator for the ListObjectsV2 operation.
	paginator := s3.NewListObjectsV2Paginator(s3Client, &params)
	filenames := make([]string, 0)
	for paginator.HasMorePages() {
		page, err := try.GetCtx(ctx, func() (*s3.ListObjectsV2Output, error) {
			return paginator.NextPage(ctx)
		}, try.WithFixedBackoff(10*time.Second))

		if err != nil {
			return filenames, err
		}
		for _, obj := range page.Contents {
			key := *obj.Key
			if p != "" {
				// Get the relative path.
				key = strings.TrimPrefix(key, p+"/")
			}
			// Skip nested directories.
			if strings.Index(key, "/") > -1 {
				continue
			}
			filenames = append(filenames, key)
		}
	}
	return filenames, nil
}

func (f *s3Adapter) Config() AdapterConfig {
	return f.AdapterConfig
}

func (f *s3Adapter) getClient(ctx context.Context) (*s3.Client, error) {
	if f.client != nil {
		return f.client, nil
	}
	cfg, err := try.GetCtx(ctx, func() (aws.Config, error) {
		return config.LoadDefaultConfig(ctx,
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(f.AccessKeyID, f.AccessSecret, "")),
			config.WithRegion(f.Region),
			config.WithRequestChecksumCalculation(0),
			config.WithResponseChecksumValidation(0),
			config.WithBaseEndpoint(f.Endpoint),
		)
	}, try.WithFixedBackoff(10*time.Second))
	if err != nil {
		return nil, err
	}

	f.client = s3.NewFromConfig(cfg)
	return f.client, nil
}

func (f *s3Adapter) joinPath(pathElem string, pathElems ...string) string {
	p := path.Join(append([]string{f.BasePath, pathElem}, pathElems...)...)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "./")
	return p
}
