package realfs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/unicode/norm"

	"dup/fs"
	"dup/lifecycle"
)

const hashFileName = ".meta.csv"
const bufSize = 256 * 1024

type meta struct {
	inode uint64
	file  *fs.FileMeta
}

type FS struct {
	root string
	idx  int
	lc   *lifecycle.Lifecycle
}

func New(path string, idx int, lc *lifecycle.Lifecycle) *FS {
	return &FS{root: path, idx: idx, lc: lc}
}

func (fsys *FS) Root() string {
	return fsys.root
}

func (fsys *FS) Scan(events fs.Events) {
	go fsys.scan(events)
}

func (fsys *FS) Sync(commands []any, events fs.Events) {
	go fsys.sync(commands, events)
}

func (fsys *FS) scan(events fs.Events) {
	fsys.lc.Started()
	defer fsys.lc.Done()

	metas := fs.FileMetas{Idx: fsys.idx}

	metaMap := fsys.readMeta()
	var metaSlice []*meta

	defer func() {
		_ = fsys.storeMeta(fsys.root, metaSlice)
		events.Send(fs.ArchiveHashed{Idx: fsys.idx})
	}()

	osfs := os.DirFS(fsys.root)
	err := iofs.WalkDir(osfs, ".", func(path string, d iofs.DirEntry, err error) error {
		if fsys.lc.ShoudStop() || !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		if err != nil {
			log.Printf("Error: failed to scan archive %q: %#v\n", fsys.root, err)
			return nil
		}

		info, err := d.Info()
		if err != nil {
			log.Printf("Error: failed to scan archive %q: %#v\n", fsys.root, err)
			return nil
		}

		size := int(info.Size())
		if size == 0 {
			return nil
		}

		modTime := info.ModTime()
		modTime = modTime.UTC().Round(time.Second)

		file := &fs.FileMeta{
			Path:    norm.NFC.String(path),
			Size:    size,
			ModTime: modTime,
		}

		sys := info.Sys().(*syscall.Stat_t)
		readMeta := metaMap[sys.Ino]
		if readMeta != nil && readMeta.ModTime == modTime && readMeta.Size == size {
			file.Hash = readMeta.Hash
		}

		metas.Metas = append(metas.Metas, *file)

		metaSlice = append(metaSlice, &meta{
			inode: sys.Ino,
			file:  file,
		})
		metaMap[sys.Ino] = file

		return nil
	})

	if err != nil {
		log.Printf("Error: failed to scan archive %q: %#v\n", fsys.root, err)
		return
	}

	events.Send(metas)

	for _, meta := range metaSlice {
		if meta.file.Hash != "" {
			continue
		}
		if fsys.lc.ShoudStop() {
			return
		}
		meta.file.Hash = fsys.hashFile(meta.file)
		events.Send(fs.FileHashed{
			Idx:  fsys.idx,
			Path: meta.file.Path,
			Hash: meta.file.Hash,
		})
	}
}

func (fsys *FS) sync(commands []any, events fs.Events) {
	defer events.Send(fs.Synced{Idx: fsys.idx})
	for _, cmd := range commands {
		switch cmd := cmd.(type) {
		case fs.Rename:
			fsys.renameFile(cmd, events)
		case fs.Copy:
			fsys.copyFile(cmd, events)
		}
	}
}

func (fsys *FS) renameFile(cmd fs.Rename, events fs.Events) {
	events.Send(fs.RenamingFile{
		Idx:  fsys.idx,
		Path: cmd.SourcePath,
	})
	path := filepath.Join(fsys.root, filepath.Dir(cmd.DestinationPath))
	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.Printf("Error: failed to create folder %q: %#v\n", path, err)
		return
	}
	from := filepath.Join(fsys.root, cmd.SourcePath)
	to := filepath.Join(fsys.root, cmd.DestinationPath)
	err = os.Rename(from, to)
	if err != nil {
		log.Printf("Error: failed to rename file %q: %#v\n", from, err)
		return
	}
	fsys.removeDirIfEmpty(filepath.Dir(from))
}

func (fsys *FS) copyFile(cmd fs.Copy, events fs.Events) {
	fsys.lc.Started()
	defer fsys.lc.Done()

	eventChans := make([]chan []byte, len(cmd.ToRoots))

	defer func() {
		for _, ch := range eventChans {
			close(ch)
		}
	}()

	source := filepath.Join(fsys.root, cmd.Path)
	info, err := os.Stat(source)
	if err != nil {
		log.Printf("Error: failed to read from file %q: %#v\n", source, err)
		return
	}

	for i, root := range cmd.ToRoots {
		eventChans[i] = make(chan []byte, 1)
		go fsys.writer(root, cmd.Path, cmd.Hash, info.Size(), info.ModTime(), eventChans[i])
	}

	sourceFile, err := os.Open(source)
	if err != nil {
		log.Printf("Error: failed to read from file %q: %#v\n", source, err)
		return
	}

	defer sourceFile.Close()

	var n int
	for err != io.EOF && !fsys.lc.ShoudStop() {
		buf := make([]byte, bufSize)
		n, err = sourceFile.Read(buf)
		if err != nil && err != io.EOF {
			log.Printf("Error: failed to read from file %q: %#v\n", source, err)
			return
		}
		for _, eventChan := range eventChans {
			eventChan <- buf[:n]
		}
		events.Send(fs.CopyingFile{
			Idx:  fsys.idx,
			Path: cmd.Path,
			Size: n,
		})
	}
}

func (fsys *FS) writer(root, path, hash string, size int64, modTime time.Time, events chan []byte) {
	fullPath := filepath.Join(root, path)
	log.Printf("writer: path %q size %d hash %q modTime %v\n", fullPath, size, hash, modTime)

	dirPath := filepath.Dir(fullPath)
	_ = os.MkdirAll(dirPath, 0755)
	file, createErr := os.Create(fullPath)
	if createErr != nil {
		log.Printf("Error: failed to create file %q: %#v\n", fullPath, createErr)
		return
	}
	var err error
	defer func() {
		if err != nil {
			log.Printf("Error: failed to write to file %q: %#v\n", fullPath, err)
			os.Remove(fullPath)
			return
		}

		if file != nil {
			info, _ := file.Stat()
			sys := info.Sys().(*syscall.Stat_t)
			newSize := info.Size()
			_ = file.Close()
			_ = os.Chtimes(fullPath, time.Now(), modTime)

			if newSize != size {
				os.Remove(fullPath)
			}

			absHashFileName := filepath.Join(root, hashFileName)
			hashInfoFile, err := os.OpenFile(absHashFileName, os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				csvWriter := csv.NewWriter(hashInfoFile)
				_ = csvWriter.Write([]string{
					fmt.Sprint(sys.Ino),
					norm.NFC.String(path),
					fmt.Sprint(size),
					modTime.UTC().Format(time.RFC3339Nano),
					hash,
				})
				csvWriter.Flush()
				_ = hashInfoFile.Close()
			}

			if fsys.lc.ShoudStop() {
				_ = os.Remove(dirPath)
			}
		}
	}()
	for buf := range events {
		if fsys.lc.ShoudStop() {
			return
		}

		n, writeErr := file.Write(buf)
		if writeErr != nil {
			err = writeErr
			return
		}
		if n < len(buf) {
			err = errors.New("Short write")
			return
		}
	}
}

func (fsys *FS) removeDirIfEmpty(path string) {
	osfs := os.DirFS(path)

	entries, _ := iofs.ReadDir(osfs, ".")
	for _, entry := range entries {
		if entry.Name() != ".DS_Store" && !strings.HasPrefix(entry.Name(), "._") {
			return
		}
	}
	os.RemoveAll(path)
}

func (fsys *FS) readMeta() map[uint64]*fs.FileMeta {
	metas := map[uint64]*fs.FileMeta{}
	absHashFileName := filepath.Join(fsys.root, hashFileName)
	hashInfoFile, err := os.Open(absHashFileName)
	if err != nil {
		return metas
	}
	defer hashInfoFile.Close()

	records, err := csv.NewReader(hashInfoFile).ReadAll()
	if err != nil || len(records) == 0 {
		return metas
	}

	for _, record := range records[1:] {
		if len(record) == 5 {
			iNode, er1 := strconv.ParseUint(record[0], 10, 64)
			path := record[1]
			size, er2 := strconv.ParseUint(record[2], 10, 64)
			modTime, er3 := time.Parse(time.RFC3339, record[3])
			modTime = modTime.UTC().Round(time.Second)
			hash := record[4]
			if hash == "" || er1 != nil || er2 != nil || er3 != nil {
				continue
			}

			metas[iNode] = &fs.FileMeta{
				Path:    path,
				Size:    int(size),
				ModTime: modTime,
				Hash:    hash,
			}

			info, ok := metas[iNode]
			if hash != "" && ok && info.ModTime == modTime && info.Size == int(size) {
				metas[iNode].Hash = hash
			}
		}
	}
	return metas
}

func (s *FS) storeMeta(root string, metas []*meta) error {
	result := make([][]string, 1, len(metas)+1)
	result[0] = []string{"INode", "Name", "Size", "ModTime", "Hash"}

	for _, meta := range metas {
		if meta.file.Hash == "" {
			continue
		}
		result = append(result, []string{
			fmt.Sprint(meta.inode),
			norm.NFC.String(meta.file.Path),
			fmt.Sprint(meta.file.Size),
			meta.file.ModTime.UTC().Format(time.RFC3339Nano),
			meta.file.Hash,
		})
	}

	absHashFileName := filepath.Join(root, hashFileName)
	hashInfoFile, err := os.Create(absHashFileName)

	if err != nil {
		return err
	}
	err = csv.NewWriter(hashInfoFile).WriteAll(result)
	_ = hashInfoFile.Close()
	return err
}

func (fsys *FS) hashFile(meta *fs.FileMeta) string {
	hash := sha256.New()
	buf := make([]byte, bufSize)
	path := filepath.Join(fsys.root, meta.Path)

	file, err := os.Open(path)
	if err != nil {
		log.Printf("Error: failed to scan archive %q: %#v\n", fsys.root, err)
		return ""
	}
	defer file.Close()

	offset := bufSize
	if meta.Size > 2*bufSize {
		offset = meta.Size - bufSize
	}
	nr, er := file.Read(buf)
	if er != nil && er != io.EOF {
		log.Printf("Error: failed to scan archive %q: %#v\n", fsys.root, err)
		return ""
	}
	hash.Write(buf[0:nr])
	if meta.Size > bufSize {
		nr, er := file.ReadAt(buf, int64(offset))
		if er != nil && er != io.EOF {
			log.Printf("Error: failed to scan archive %q: %#v\n", fsys.root, err)
			return ""
		}
		hash.Write(buf[0:nr])
	}

	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
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
