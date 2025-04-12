package fs

import "time"

type Events interface {
	Send(msg any)
}

type FS interface {
	Root() string
	Scan(events Events)
	Sync(commands []any, events Events)
}

// Events

type FileMetas struct {
	Idx   int
	Metas []FileMeta
}
type FileMeta struct {
	Idx     int
	Path    string
	Size    int
	ModTime time.Time
	Hash    string
}

type FileHashed struct {
	Idx  int
	Path string
	Hash string
}

type ArchiveHashed struct {
	Idx int
}
