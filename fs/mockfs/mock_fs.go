package mockfs

import (
	"cmp"
	"dup/fs"
	"dup/lifecycle"
	"encoding/csv"
	"log"
	"os"
	"slices"
	"strconv"
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

func (fsys *FS) Root() string {
	return fsys.path
}

func (fsys *FS) Scan(events fs.Events) {
	go fsys.scan(events)
}

func (fsys *FS) Sync(commands []any, events fs.Events) {
	go fsys.sync(commands, events)
}

func (fsys *FS) scan(events fs.Events) {
	time.Sleep(time.Second * time.Duration(fsys.idx))
	metas := []fs.FileMeta{}
	for _, meta := range archives[fsys.path] {
		meta.Hash = ""
		metas = append(metas, meta)
	}
	events.Send(fs.FileMetas{
		Idx:   fsys.idx,
		Metas: metas,
	})
	metas = archives[fsys.path]
	for _, file := range metas {
		events.Send(fs.FileHashed{
			Idx:  fsys.idx,
			Path: file.Path,
			Hash: file.Hash,
		})
		time.Sleep(time.Millisecond)
	}

	events.Send(fs.ArchiveHashed{Idx: fsys.idx})
}

func (fsys *FS) sync(commands []any, events fs.Events) {
	for _, command := range commands {
		log.Printf("FS: command %#v\n", command)
		switch cmd := command.(type) {
		case fs.Rename:
			events.Send(fs.RenamingFile{
				Idx:  fsys.idx,
				Path: cmd.DestinationPath,
			})
		case fs.Copy:
			size := 0
			for _, file := range archives["origin"] {
				if file.Path == cmd.Path {
					size = file.Size
					break
				}
			}
			progress := 0
			for {
				delta := 100_000
				if delta > size-progress {
					delta = size - progress
				}
				if fsys.lc.ShoudStop() {
					return
				}
				events.Send(fs.CopyingFile{
					Idx:  fsys.idx,
					Path: cmd.Path,
					Size: delta,
				})
				time.Sleep(time.Millisecond)
				progress += delta
				if progress >= size {
					break
				}
			}
		}
	}
	events.Send(fs.Synced{
		Idx: fsys.idx,
	})
}

var archives = map[string][]fs.FileMeta{}

func init() {
	or := readMeta()
	// or := []fs.FileMeta{}

	c1 := slices.Clone(or)
	c2 := slices.Clone(or)

	or = append(or, fs.FileMeta{
		Path:    "aaa/bbb/ccc",
		Size:    11111111,
		ModTime: time.Now(),
		Hash:    "ccc",
	})

	or = append(or, fs.FileMeta{
		Path:    "bbb",
		Size:    12300000,
		ModTime: time.Now(),
		Hash:    "bbb",
	})

	or = append(or, fs.FileMeta{
		Path:    "ccc",
		Size:    12356700,
		ModTime: time.Now(),
		Hash:    "ccc",
	})

	or = append(or, fs.FileMeta{
		Path:    "yyy",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	or = append(or, fs.FileMeta{
		Path:    "zzz",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	or = append(or, fs.FileMeta{
		Path:    "nnn/mmm1/aaa",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "nnn/mmm1/aaa",
	})

	or = append(or, fs.FileMeta{
		Path:    "nnn/mmm1/bbb",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "nnn/mmm1/bbb",
	})

	or = append(or, fs.FileMeta{
		Path:    "nnn/mmm1/ccc",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "nnn/mmm1/ccc",
	})

	or = append(or, fs.FileMeta{
		Path:    "nnn/mmm2/aaa",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "nnn/mmm2/aaa",
	})

	or = append(or, fs.FileMeta{
		Path:    "nnn/mmm2/bbb",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "nnn/mmm2/bbb",
	})

	or = append(or, fs.FileMeta{
		Path:    "ddd",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "ddd",
	})

	c1 = append(c1, fs.FileMeta{
		Path:    "ccc",
		Size:    12300000,
		ModTime: time.Now(),
		Hash:    "bbb",
	})

	c1 = append(c1, fs.FileMeta{
		Path:    "bbb",
		Size:    12356700,
		ModTime: time.Now(),
		Hash:    "ccc",
	})

	c1 = append(c1, fs.FileMeta{
		Path:    "xxx",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	c1 = append(c1, fs.FileMeta{
		Path:    "yyy",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	c1 = append(c1, fs.FileMeta{
		Path:    "zzz",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	c2 = append(c2, fs.FileMeta{
		Path:    "aaa/bbb",
		Size:    23400000,
		ModTime: time.Now(),
		Hash:    "222",
	})

	c2 = append(c2, fs.FileMeta{
		Path:    "ddd/eee",
		Size:    12300000,
		ModTime: time.Now(),
		Hash:    "111",
	})

	c2 = append(c2, fs.FileMeta{
		Path:    "ddd/fff",
		Size:    33300000,
		ModTime: time.Now(),
		Hash:    "333",
	})

	c2 = append(c2, fs.FileMeta{
		Path:    "xxx",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	c2 = append(c2, fs.FileMeta{
		Path:    "yyy",
		Size:    99900000,
		ModTime: time.Now(),
		Hash:    "xxx",
	})

	for i := range or {
		or[i].Idx = 0
	}
	for i := range c1 {
		c1[i].Idx = 1
	}
	for i := range c2 {
		c2[i].Idx = 2
	}

	archives = map[string][]fs.FileMeta{
		"origin": or,
		"copy 1": c1,
		"copy 2": c2,
	}
}

func readMeta() []fs.FileMeta {
	result := []fs.FileMeta{}
	hashInfoFile, err := os.Open("data/.meta.csv")
	if err != nil {
		return nil
	}
	defer hashInfoFile.Close()

	records, err := csv.NewReader(hashInfoFile).ReadAll()
	if err != nil || len(records) == 0 {
		return nil
	}

	for _, record := range records[1:] {
		if len(record) == 5 {
			name := record[1]
			size, er2 := strconv.ParseUint(record[2], 10, 64)
			modTime, er3 := time.Parse(time.RFC3339, record[3])
			modTime = modTime.UTC().Round(time.Second)
			hash := record[4]
			if hash == "" || er2 != nil || er3 != nil {
				continue
			}

			result = append(result, fs.FileMeta{
				Path:    name,
				Hash:    hash,
				Size:    int(size),
				ModTime: modTime,
			})
		}
	}
	slices.SortFunc(result, func(a, b fs.FileMeta) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return result
}
