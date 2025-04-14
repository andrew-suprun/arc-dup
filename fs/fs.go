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

// Commands

type Copy struct {
	Path    string
	Hash    string
	ToRoots []string
}

type Rename struct {
	SourcePath      string
	DestinationPath string
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

type RenamingFile struct {
	Idx  int
	Path string
}

type CopyingFile struct {
	Idx  int
	Path string
	Size int
}

type Synced struct {
	Idx int
}
