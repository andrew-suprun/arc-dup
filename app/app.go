package app

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"dup/fs"
	"dup/lifecycle"
)

func Run(fss []fs.FS, lc *lifecycle.Lifecycle) {
	m := make(model, 1)
	p := tea.NewProgram(m)

	archives := make([]*archive, len(fss))
	for i, fs := range fss {
		archives[i] = &archive{fs: fs, files: map[string]*file{}}
	}

	app := &app{
		archives:        archives,
		lc:              lc,
		events:          events{p},
		backup:          fmt.Sprintf("~~~%s~~~", time.Now().UTC().Format(time.RFC3339)),
		syncingArchives: len(archives) - 1,
	}

	for _, fs := range fss {
		fs.Scan(app.events)
	}

	m <- app

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type events struct {
	p *tea.Program
}

func (e events) Send(event any) {
	e.p.Send(event)
}

type model chan *app

func (m model) Init() tea.Cmd {
	return nil
}

type quit struct{}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	app := <-m
	defer func() { m <- app }()

	switch msg := msg.(type) {
	case quit:
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			go func() {
				app.lc.Stop()
				app.events.Send(quit{})
			}()
			return m, nil
		}

	case fs.FileMetas:
		archive := app.archives[msg.Idx]
		for _, meta := range msg.Metas {
			archive.files[meta.Path] = &file{
				path:    meta.Path,
				size:    meta.Size,
				modTime: meta.ModTime,
				hash:    meta.Hash,
			}
			if meta.Hash == "" {
				archive.size++
			}
		}

	case fs.FileHashed:
		archive := app.archives[msg.Idx]
		file := archive.findFile(msg.Path)
		file.hash = msg.Hash
		archive.done++
		archive.state = hashing

	case fs.ArchiveHashed:
		archive := app.archives[msg.Idx]
		archive.state = hashed
		allHashed := true
		for _, archive := range app.archives {
			if archive.state != hashed {
				allHashed = false
			}
		}
		if allHashed {
			app.analyzeArchives()
			app.state = appRenaming
			for _, archive := range app.archives[1:] {
				archive.done = 0
				archive.size = len(archive.commands)
				archive.fs.Sync(archive.commands, app.events)
			}
		}

	case fs.RenamingFile:
		archive := app.archives[msg.Idx]
		archive.done++

	case fs.CopyingFile:
		archive := app.archives[msg.Idx]
		archive.done += msg.Size
		file := archive.files[msg.Path]
		if archive.filePath != file.path {
			archive.filePath = file.path
			archive.fileSize = file.size
			archive.fileCopyed = msg.Size
		} else {
			archive.fileCopyed += msg.Size
			if archive.fileCopyed >= file.size {
				archive.fileCopyed = file.size
				archive.done += file.size - archive.fileCopyed
			}
		}

	case fs.Synced:
		if app.state == appCopying {
			app.state = appDone
			app.lc.Stop()
			return m, func() tea.Msg { return quit{} }
		}
		app.syncingArchives--
		if app.syncingArchives == 0 {
			archive := app.archives[0]
			for _, cmd := range archive.commands {
				path := cmd.(fs.Copy).Path
				file := archive.files[path]
				archive.size += file.size
			}
			archive.done = 0
			app.state = appCopying
			archive.fs.Sync(archive.commands, app.events)
		}
	}
	return m, nil
}

func (m model) View() string {
	app := <-m
	b := strings.Builder{}
	switch app.state {
	case appStarted:
		for _, archive := range app.archives {
			switch archive.state {
			case scanning:
				fmt.Fprintf(&b, "scanning:         %s\n", archive.fs.Root())
			case hashing:
				var done float64 = 100
				if archive.size > 0 {
					done = float64(archive.done) * 100 / float64(archive.size)
				}
				fmt.Fprintf(&b, "hashing  %6.2f%%: %s\n", done, archive.fs.Root())
			case hashed:
				fmt.Fprintf(&b, "hashed:           %s\n", archive.fs.Root())
			}
		}
	case appRenaming:
		for i, archive := range app.archives {
			if i == 0 {
				fmt.Fprintf(&b, "waiting:           %s\n", archive.fs.Root())
				continue
			}
			var done float64 = 100
			if archive.size > 0 {
				done = float64(archive.done) * 100 / float64(archive.size)
			}
			fmt.Fprintf(&b, "renaming %6.2f%%: %s\n", done, archive.fs.Root())
			fmt.Fprintf(&b, "    file        : %s\n", archive.filePath)
		}

	case appCopying:
		archive := app.archives[0]
		var done float64 = 100
		if archive.size > 0 {
			done = float64(archive.done) * 100 / float64(archive.size)
		}
		var fileDone float64 = 100
		if archive.fileSize > 0 {
			fileDone = float64(archive.fileCopyed) * 100 / float64(archive.fileSize)
		}
		dir, file := filepath.Split(archive.filePath)
		fmt.Fprintf(&b, "copying  %6.2f%%: %s/%s\n", done, archive.fs.Root(), dir)
		fmt.Fprintf(&b, "   file  %6.2f%%: %s\n", fileDone, file)
	}
	m <- app
	return b.String()
}

func (app *app) analyzeArchives() {
	app.ignoreIdenticalFiles()
	app.backupExcessFiles()
	app.resolveConflicts()
	app.renameAndCopyFiles()

	commands := app.archives[0].commands
	sort.Slice(commands, func(i, j int) bool {
		iCmd := commands[i].(fs.Copy)
		jCmd := commands[j].(fs.Copy)
		return iCmd.Path < jCmd.Path
	})
}

func (app *app) ignoreIdenticalFiles() {
	identicalFiles := []string{}
	for _, original := range app.archives[0].files {
		hasIdentical := true
		for _, archive := range app.archives[1:] {
			copy, ok := archive.files[original.path]
			if !ok || original.size != copy.size || original.hash != copy.hash {
				hasIdentical = false
			}
		}
		if hasIdentical {
			identicalFiles = append(identicalFiles, original.path)
		}
	}
	for _, archive := range app.archives {
		for _, path := range identicalFiles {
			delete(archive.files, path)
		}
	}
}

func (app *app) backupExcessFiles() {
	hashes := map[string]struct{}{}
	for _, archive := range app.archives {
		for _, file := range archive.files {
			hashes[file.hash] = struct{}{}
		}
	}

	originals := app.archives[0].byHash()
	for _, archive := range app.archives[1:] {
		copies := archive.byHash()
		for hash := range hashes {
			originalFiles := originals[hash]
			copyFiles := copies[hash]

			if len(originalFiles) >= len(copyFiles) {
				continue
			}
			for i := len(originalFiles); i < len(copyFiles); i++ {
				path := copyFiles[i]
				archive.commands = append(archive.commands, fs.Rename{
					SourcePath:      path,
					DestinationPath: filepath.Join(app.backup, path),
				})
				delete(archive.files, path)
			}
		}
	}
}

func (app *app) resolveConflicts() {
	for _, file := range app.archives[0].files {
		for _, archive := range app.archives[1:] {
			if other, ok := archive.files[file.path]; ok {
				dir, name := filepath.Split(other.path)
				newPath := filepath.Join(dir, app.backup+name)
				archive.commands = append(archive.commands, fs.Rename{
					SourcePath:      other.path,
					DestinationPath: newPath,
				})
				other.path = newPath
				archive.files[newPath] = other
				delete(archive.files, file.path)
			}
		}
	}
}

func (app *app) renameAndCopyFiles() {
	toCopy := map[string][]string{}
	originalsByHash := app.archives[0].byHash()
	for _, archive := range app.archives[1:] {
		copiesByHash := archive.byHash()
		for hash, originals := range originalsByHash {
			copies := copiesByHash[hash]
			for i, original := range originals {
				if i < len(copies) {
					archive.commands = append(archive.commands, fs.Rename{
						SourcePath:      copies[i],
						DestinationPath: original,
					})
				} else {
					newRoots := toCopy[original]
					newRoots = append(newRoots, archive.fs.Root())
					toCopy[original] = newRoots
				}
			}
		}
	}
	for path, roots := range toCopy {
		archive := app.archives[0]
		if len(roots) > 0 {
			archive.commands = append(archive.commands, fs.Copy{
				Path:    path,
				Hash:    archive.files[path].hash,
				ToRoots: roots,
			})
		}

	}
}

func (arc *archive) byHash() map[string][]string {
	result := map[string][]string{}
	for _, file := range arc.files {
		paths := result[file.hash]
		paths = append(paths, file.path)
		result[file.hash] = paths
	}
	return result
}

type appState int

const (
	appStarted appState = iota
	appRenaming
	appCopying
	appDone
)

type archiveState int

const (
	scanning archiveState = iota
	hashing
	hashed
	renaming
)

type app struct {
	state           appState
	archives        []*archive
	lc              *lifecycle.Lifecycle
	events          events
	backup          string
	syncingArchives int
}

type archive struct {
	state      archiveState
	fs         fs.FS
	files      files
	commands   []any
	size       int
	done       int
	filePath   string
	fileSize   int
	fileCopyed int
}

type file struct {
	path    string
	size    int
	modTime time.Time
	hash    string
}

type files map[string]*file

func (arc *archive) findFile(path string) *file {
	for _, file := range arc.files {
		if file.path == path {
			return file
		}
	}
	return nil
}
