package mockfs

import (
	"dup/fs"
	"dup/lifecycle"
	"time"
)

type FS struct {
	path string
	idx  int
	lc   *lifecycle.Lifecycle
}

func New(path string, idx int, lc *lifecycle.Lifecycle) *FS {
	return &FS{path: path, idx: idx, lc: lc}
}

func (fsys *FS) Scan(events fs.Events) {
	go func() {
		for i := range 30 {
			time.Sleep(time.Millisecond * 100)
			events.Send(fs.EventDebugScan{Idx: fsys.idx, N: i})
		}
	}()
}

func (fs *FS) Sync(commands []any, events fs.Events) {
}
