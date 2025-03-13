package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sin/internal/utils"
)

var _ Adapter = (*fileAdapter)(nil)

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
	err := utils.CopyFile(ctx, source, dest)
	if err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func (f *fileAdapter) Del(_ context.Context, pathElem string, pathElems ...string) error {
	path := filepath.Join(append([]string{pathElem}, pathElems...)...)
	return os.Remove(path)
}

func (f *fileAdapter) ListFileNames(_ context.Context, pathElems ...string) ([]string, error) {
	path := filepath.Join(append([]string{f.Dir}, pathElems...)...)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]string, 0)
	err := filepath.Walk(path, func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (f *fileAdapter) Config() AdapterConfig {
	return f.AdapterConfig
}
