package store

import (
	"context"
	"errors"
)

const (
	AdapterS3Type   = "s3"
	AdapterFileType = "file"
	AdapterMockType = "mock"
)

// Adapter abstract storage adapter.
type Adapter interface {
	// Save saves a file to the storage, override if the file already exists.
	// If extra pathElems are given, pathElems will be joined.
	Save(ctx context.Context, source string, pathElem string, pathElems ...string) error

	// Del removes a file from the storage.
	// If extra pathElems are given, pathElems will be joined.
	// Do nothing if the file is directory.
	Del(ctx context.Context, pathElem string, pathElems ...string) error

	// ListFileNames return list of file names in the given path.
	// Return empty if not a directory, pathElems will be joined.
	ListFileNames(ctx context.Context, pathElems ...string) ([]string, error)

	Config() AdapterConfig

	Type() string
}

var (
	ErrFileNotFound = errors.New("file not found")
)

// Downloader Adapter that can download a file.
type Downloader interface {
	Adapter
	// Download downloads a file from the storage.
	// By default, it searches for a file named by destination.
	// If sourcePaths are given, it will search for a file named by sourcePaths joined.
	Download(ctx context.Context, destination string, sourcePaths ...string) error
}

type AdapterConfig struct {
	Name string `json:"name"`

	// Disabled whether this adapter should be skipped.
	Disabled bool `json:"disabled"`

	// Keep override the Syncer Keep. Default 0 (using the Syncer Keep).
	Keep int `json:"keep"`

	// Each controls the number of actual syncs.
	// Default it will sync every backup.
	// If set to number n > 1, it will sync every nth backup.
	Each int `json:"each"`
}
