package utils

import (
	"context"
	"io"
	"os"
)

type readerFunc func(p []byte) (n int, err error)

func (rf readerFunc) Read(p []byte) (n int, err error) { return rf(p) }

// CopyFile copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all its contents will be replaced by the contents
// of the source file.
// TODO: review this copy code.
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
