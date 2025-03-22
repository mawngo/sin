package utils

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/samber/lo"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sin/internal/core"
	"slices"
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

func ListFileNames(path string) ([]string, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}

	return files, err
}

// FilterBackupFileNames filters out non-managed backup files,
// and sorts the remaining result based on alphabetical order.
func FilterBackupFileNames(names []string, filename string) []string {
	if len(names) == 0 {
		return names
	}
	reg, err := regexp.Compile(fmt.Sprintf(`\d{6}_\d{4}_%s%s$`, filename, core.BackupFileExt))
	if err != nil {
		slog.Error("error compiling regexp", slog.String("filename", filename), slog.Any("err", err))
		panic(fmt.Errorf("error compiling regexp: %w", err))
	}
	names = lo.Filter(names, func(name string, _ int) bool {
		return reg.MatchString(name)
	})
	slices.Sort(names)
	return names
}

func DelFile(path string) error {
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		if errors.Is(err, os.ErrNotExist) || info.IsDir() {
			return nil
		}
		return err
	}
	checksum := path + ChecksumExt
	err := os.Remove(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(checksum); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.Remove(checksum)
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
