package realfs

import (
	"os"
	"path/filepath"

	"golang.org/x/text/unicode/norm"

	"dup/fs"
	"dup/lifecycle"
)

type FS struct {
	path string
	idx  int
	lc   *lifecycle.Lifecycle
}

func New(path string, idx int, lc *lifecycle.Lifecycle) *FS {
	return &FS{path: path, idx: idx, lc: lc}
}

func (fsys *FS) Root() string {
	return fsys.path
}

func (fsys *FS) Scan(events fs.Events) {
}

func (fs *FS) Sync(commands []any, events fs.Events) {
}

func AbsPath(path string) (string, error) {
	var err error
	path, err = filepath.Abs(path)
	path = norm.NFC.String(path)
	if err != nil {
		return "", err
	}

	_, err = os.Stat(path)
	if err != nil {
		return "", err
	}
	return path, nil
}
