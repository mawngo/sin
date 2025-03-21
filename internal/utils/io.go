package utils

import (
	"context"
	"encoding/hex"
	"io"
	"os"
)

const ChecksumExt = ".sha256.txt"

type readerFunc func(p []byte) (n int, err error)

func (rf readerFunc) Read(p []byte) (n int, err error) { return rf(p) }

func CopyFile(ctx context.Context, src string, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, readerFunc(func(p []byte) (int, error) {
		// Wrapper for allowing context cancellation.
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			return in.Read(p)
		}
	}))
	if err != nil {
		return err
	}
	return out.Sync()
}

func CreateFileSHA256Checksum(path string, dest ...string) error {
	// Write the checksum file first.
	checksum, err := FileSHA256Checksum(path)
	if err != nil {
		return err
	}
	destChecksum := path + ChecksumExt
	if len(dest) > 0 {
		destChecksum = dest[0]
	}

	err = (func() error {
		fi, err := os.Create(destChecksum)
		if err != nil {
			return err
		}
		defer fi.Close()
		_, err = fi.WriteString(hex.EncodeToString(checksum))
		return err
	})()
	return err
}
