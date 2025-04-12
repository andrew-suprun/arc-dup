package realfs

import (
	"os"
	"path/filepath"

	"golang.org/x/text/unicode/norm"

	"dup/lifecycle"
)

type FS struct {
	path     string
	lc       *lifecycle.Lifecycle
	commands chan any
	events   chan any
}

func New(path string, lc *lifecycle.Lifecycle) *FS {
	return &FS{
		path:     path,
		lc:       lc,
		commands: make(chan any, 1),
		events:   make(chan any, 10),
	}
}

func (fs *FS) Commands() chan<- any {
	return fs.commands
}

func (fs *FS) Events() <-chan any {
	return fs.events
}

func (fs *FS) Run() {}

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
