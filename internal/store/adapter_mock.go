package store

import (
	"bufio"
	"context"
	"github.com/mawngo/go-errors"
	"github.com/samber/lo"
	"os"
	"path"
	"path/filepath"
	"sin/internal/utils"
	"slices"
	"strings"
)

var _ Adapter = (*mockAdapter)(nil)
var _ Downloader = (*mockAdapter)(nil)

// mockAdapter only write results into a log file.
// fileAdapter is not safe for concurrent use.
type mockAdapter struct {
	AdapterConfig
	Dir         string `json:"dir"`
	LogFilename string `json:"logFilename"`
}

func (m *mockAdapter) Type() string {
	return AdapterMockType
}

func newMockAdapter(conf map[string]any) (Adapter, error) {
	adapter := mockAdapter{}
	if err := utils.MapToStruct(conf, &adapter); err != nil {
		return nil, err
	}
	if adapter.Name == "" {
		adapter.Name = adapter.Type()
	}
	if adapter.Dir == "" {
		adapter.Dir = "."
	}
	if adapter.Dir != "." {
		_ = os.MkdirAll(adapter.Dir, os.ModePerm)
	}
	if adapter.LogFilename == "" {
		adapter.LogFilename = AdapterMockType + ".remote.log"
	}
	return &adapter, nil
}

func (m *mockAdapter) Save(_ context.Context, _ string, pathElem string, pathElems ...string) error {
	filename := m.joinPath(pathElem, pathElems...)
	files, err := m.openLog(m.LogFilename)
	if err != nil {
		return err
	}
	checksumFile := filename + utils.ChecksumExt
	files = lo.Filter(files, func(file string, _ int) bool {
		return file != filename && file != checksumFile
	})
	files = append(files, filename, checksumFile)
	return m.writeLog(m.LogFilename, files)
}

func (m *mockAdapter) Del(_ context.Context, pathElem string, pathElems ...string) error {
	filename := m.joinPath(pathElem, pathElems...)
	files, err := m.openLog(m.LogFilename)
	if err != nil {
		return err
	}
	checksumFile := filename + utils.ChecksumExt
	files = lo.Filter(files, func(file string, _ int) bool {
		return file != filename && file != checksumFile
	})
	return m.writeLog(m.LogFilename, files)
}

func (m *mockAdapter) ListFileNames(_ context.Context, pathElems ...string) ([]string, error) {
	prefix := m.joinPath("", pathElems...)
	files, err := m.openLog(m.LogFilename)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return files, nil
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return lo.Filter(files, func(file string, _ int) bool {
		return !strings.HasPrefix(file, prefix)
	}), nil
}

func (m *mockAdapter) Download(_ context.Context, destination string, sourcePaths ...string) error {
	if len(sourcePaths) == 0 {
		sourcePaths = []string{filepath.Base(destination)}
	}
	source := m.joinPath("", sourcePaths...)
	files, err := m.openLog(m.LogFilename)
	if err != nil {
		return err
	}
	if !slices.Contains(files, source) {
		return errors.Wrapf(ErrFileNotFound, "file %s not found", source)
	}

	f, err := os.Create(destination)
	if err != nil {
		return errors.Wrapf(err, "error creating file %s", destination)
	}
	f.Close()

	// Optionally, handling checksum verification.
	sourceChecksum := source + utils.ChecksumExt
	if slices.Contains(files, sourceChecksum) {
		return utils.CreateFileSHA256Checksum(destination, destination+utils.ChecksumExt)
	}
	return nil
}

func (m *mockAdapter) Config() AdapterConfig {
	return m.AdapterConfig
}

func (m *mockAdapter) openLog(filenames ...string) ([]string, error) {
	res := make([]string, 0, max(m.Keep, 10))
	for _, filename := range filenames {
		err := (func() error {
			file, err := os.Open(filepath.Join(m.Dir, filename))
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return errors.Wrapf(err, "error opening log file %s", filename)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				content := strings.TrimSpace(scanner.Text())
				if content != "" {
					res = append(res, content)
				}
			}
			if err := scanner.Err(); err != nil {
				return errors.Wrapf(err, "error reading log file %s", filename)
			}
			return nil
		})()
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (m *mockAdapter) writeLog(filename string, content []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return errors.Wrapf(err, "error creating file %s", filename)
	}
	defer file.Close()

	for _, c := range content {
		if _, err := file.WriteString(c + "\n"); err != nil {
			return errors.Wrapf(err, "error writing file %s", filename)
		}
	}
	return nil
}

func (m *mockAdapter) joinPath(pathElem string, pathElems ...string) string {
	p := path.Join(append([]string{pathElem}, pathElems...)...)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "./")
	return p
}
