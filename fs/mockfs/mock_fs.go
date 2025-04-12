package mockfs

import "dup/lifecycle"

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
