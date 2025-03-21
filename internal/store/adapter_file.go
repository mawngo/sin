package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sin/internal/utils"
)

var _ Adapter = (*fileAdapter)(nil)

// fileAdapter is a local file adapter.
// fileAdapter is not safe for concurrent use.
type fileAdapter struct {
	AdapterConfig
	Dir string `json:"dir"`
}

func newFileAdapter(conf map[string]any) (Adapter, error) {
	adapter := fileAdapter{}
	if err := utils.MapToStruct(conf, &adapter); err != nil {
		return nil, err
	}
	if adapter.Dir == "" {
		return nil, errors.New("missing dir config for file adapter " + adapter.Name)
	}
	return &adapter, nil
}

func (f *fileAdapter) Save(ctx context.Context, source string, pathElem string, pathElems ...string) error {
	dest := filepath.Join(append([]string{f.Dir, pathElem}, pathElems...)...)
	if err := os.MkdirAll(filepath.Dir(dest), os.ModePerm); err != nil {
		return err
	}

	destChecksum := dest + utils.ChecksumExt
	err := utils.CreateFileSHA256Checksum(source, destChecksum)

	err = utils.CopyFile(ctx, source, dest)
	if err != nil {
		_ = os.Remove(dest)
		_ = os.Remove(destChecksum)
		return err
	}
	return nil
}

func (f *fileAdapter) Del(_ context.Context, pathElem string, pathElems ...string) error {
	path := filepath.Join(append([]string{f.Dir, pathElem}, pathElems...)...)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		if errors.Is(err, os.ErrNotExist) || info.IsDir() {
			return nil
		}
		return err
	}
	checksum := path + utils.ChecksumExt
	err := os.Remove(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(checksum); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.Remove(checksum)
}

func (f *fileAdapter) ListFileNames(_ context.Context, pathElems ...string) ([]string, error) {
	path := filepath.Join(append([]string{f.Dir}, pathElems...)...)
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

func (f *fileAdapter) Config() AdapterConfig {
	return f.AdapterConfig
}
