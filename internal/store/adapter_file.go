package store

import (
	"context"
	"errors"
	"github.com/mitchellh/mapstructure"
	"io/fs"
)

var _ Adapter = (*fileAdapter)(nil)

type fileAdapter struct {
	AdapterConfig
	Dir string `json:"dir"`
}

func newFileAdapter(conf map[string]any) (Adapter, error) {
	adapter := fileAdapter{}
	if err := mapstructure.Decode(conf, &adapter); err != nil {
		return nil, err
	}
	if adapter.Dir != "" {
		return nil, errors.New("missing dir config for file adapter " + adapter.Name)
	}
	return &adapter, nil
}

func (f fileAdapter) Save(ctx context.Context, file fs.File, pathElem string, pathElems ...string) error {
	//TODO implement me
	panic("implement me")
}

func (f fileAdapter) Del(ctx context.Context, pathElem string, pathElems ...string) error {
	//TODO implement me
	panic("implement me")
}

func (f fileAdapter) ListFileNames(ctx context.Context, pathElems ...string) ([]string, error) {
	//TODO implement me
	panic("implement me")
}

func (f fileAdapter) Config() AdapterConfig {
	//TODO implement me
	panic("implement me")
}
