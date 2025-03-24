package store

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/mawngo/go-errors"
	"github.com/mawngo/go-try/v2"
	"os"
	"path"
	"path/filepath"
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
var _ Downloader = (*s3Adapter)(nil)

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

	client *s3.Client
}

func (f *s3Adapter) Type() string {
	return AdapterS3Type
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
	if adapter.Name == "" {
		adapter.Name = adapter.Type()
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
		return errors.Wrapf(err, "error calculating checksum file %s", source)
	}
	file, err := os.Open(source)
	if err != nil {
		return errors.Wrapf(err, "error opening file %s", source)
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return errors.Wrapf(err, "error getting file info %s", source)
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
		return errors.Wrapf(err, "error uploading %s", p)
	}

	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx,
		&s3.HeadObjectInput{Bucket: aws.String(f.Bucket), Key: aws.String(p)},
		5*time.Minute)
	if err != nil {
		return errors.Wrapf(err, "error waiting for object %s", p)
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
		return errors.Wrapf(err, "error uploading %s", p)
	}
	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx,
		&s3.HeadObjectInput{Bucket: aws.String(f.Bucket), Key: aws.String(p)},
		5*time.Minute)
	if err != nil {
		return errors.Wrapf(err, "error waiting for object %s", p)
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
		return errors.Wrapf(err, "error uploadingchecksum %s", p)
	}
	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx,
		&s3.HeadObjectInput{Bucket: aws.String(f.Bucket), Key: aws.String(p)},
		5*time.Minute)
	if err != nil {
		return errors.Wrapf(err, "error waiting for checksum %s", p)
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
			if strings.Contains(key, "/") {
				continue
			}
			filenames = append(filenames, key)
		}
	}
	return filenames, nil
}

func (f *s3Adapter) Download(ctx context.Context, destination string, sourcePaths ...string) error {
	s3Client, err := f.getClient(ctx)
	if err != nil {
		return err
	}

	if len(sourcePaths) == 0 {
		sourcePaths = []string{filepath.Base(destination)}
	}
	source := f.joinPath("", sourcePaths...)
	res, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(f.Bucket),
		Key:    aws.String(source),
	})
	if err != nil {
		return errors.Wrapf(err, "error head file %s", source)
	}
	if res.ContentLength == nil {
		return errors.New("cannot determine file size")
	}

	if err := f.downloadChecksum(ctx, s3Client, destination, source); err != nil {
		return err
	}

	if *res.ContentLength < int64(f.Multipart.ThresholdMB*MB) {
		err = f.download(ctx, s3Client, destination, source)
	} else {
		err = f.downloadMultipart(ctx, s3Client, destination, source)
	}
	if err != nil {
		return err
	}
	return utils.VerifyFileSHA256Checksum(destination)
}

func (f *s3Adapter) download(ctx context.Context, s3Client *s3.Client, destination string, source string) error {
	result, err := try.GetCtx(ctx, func() (*s3.GetObjectOutput, error) {
		return s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(f.Bucket),
			Key:    aws.String(source),
		})
	}, try.WithFixedBackoff(10*time.Second))
	if err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			return ErrFileNotFound
		}
		return errors.Wrapf(err, "error downloading file %s", source)
	}
	defer result.Body.Close()
	if err := utils.CopyToFile(ctx, result.Body, destination); err != nil {
		return errors.Wrapf(err, "error writing file %s", destination)
	}
	return nil
}

func (f *s3Adapter) downloadMultipart(ctx context.Context, s3Client *s3.Client, destination string, source string) (err error) {
	downloader := manager.NewDownloader(s3Client, func(u *manager.Downloader) {
		u.PartSize = int64(min(f.Multipart.PartSizeMB, 10) * MB)
		u.Concurrency = f.Multipart.Concurrency
	})

	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	// TODO: should we retry this?
	_, err = downloader.Download(ctx, out, &s3.GetObjectInput{
		Bucket: aws.String(f.Bucket),
		Key:    aws.String(source),
	})

	if err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			return ErrFileNotFound
		}
		return errors.Wrapf(err, "error downloading file %s", source)
	}

	return out.Sync()
}

func (f *s3Adapter) downloadChecksum(ctx context.Context, s3Client *s3.Client, destination string, source string) error {
	destination += utils.ChecksumExt
	source += utils.ChecksumExt
	err := f.download(ctx, s3Client, destination, source)
	if errors.Is(err, ErrFileNotFound) {
		return nil
	}
	return errors.Wrapf(err, "error downloading checksum file %s", source)
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
		return nil, errors.Wrapf(err, "error loading aws config")
	}

	f.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.DisableLogOutputChecksumValidationSkipped = true
	})
	return f.client, nil
}

func (f *s3Adapter) joinPath(pathElem string, pathElems ...string) string {
	p := path.Join(append([]string{f.BasePath, pathElem}, pathElems...)...)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "./")
	return p
}
