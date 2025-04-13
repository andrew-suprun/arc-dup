package app

import (
	"fmt"
	"log"
	"path/filepath"
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
		archives: archives,
		lc:       lc,
		events:   events{p},
		backup:   fmt.Sprintf("~~~%s~~~", time.Now().UTC().Format(time.RFC3339)),
	}
	m <- app

	for _, fs := range fss {
		fs.Scan(app.events)
	}

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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		log.Printf("key %s", msg.String())
		switch msg.String() {
		case "esc":
			// TODO: Graceful shutdon
			return m, tea.Quit
		}

	case fs.FileMetas:
		state := <-m
		archive := state.archives[msg.Idx]
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
		m <- state

	case fs.FileHashed:
		state := <-m
		archive := state.archives[msg.Idx]
		file := archive.findFile(msg.Path)
		file.hash = msg.Hash
		archive.done++
		archive.archiveState = archiveScanned
		m <- state

	case fs.ArchiveHashed:
		app := <-m
		archive := app.archives[msg.Idx]
		archive.archiveState = archiveHashed
		allHashed := true
		for _, archive := range app.archives {
			if archive.archiveState != archiveHashed {
				allHashed = false
			}
		}
		if allHashed {
			app.analyzeArchives()
		}
		m <- app
	}
	return m, nil
}

func (m model) View() string {
	app := <-m
	b := strings.Builder{}
	b.WriteString("Analyzing archives:\n")
	for _, archive := range app.archives {
		switch archive.archiveState {
		case archiveStarted:
			fmt.Fprintf(&b, "scanning:        %s\n", archive.fs.Root())
		case archiveScanned:
			var done float64 = 100
			if archive.size > 0 {
				done = float64(archive.done) * 100 / float64(archive.size)
			}
			fmt.Fprintf(&b, "hashing %6.2f%%: %s\n", done, archive.fs.Root())
		case archiveHashed:
			fmt.Fprintf(&b, "hashed:          %s\n", archive.fs.Root())
		}
	}
	m <- app
	return b.String()
}

func (app *app) analyzeArchives() {
	app.ignoreIdenticalFiles()
	app.backupExcessFiles()
	app.resolveConflicts()
	app.renameFiles()
	app.copyFiles()

	for i, archive := range app.archives {
		log.Println("---", i)
		for _, command := range archive.commands {
			log.Printf("  %#v\n", command)
		}
	}
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
	for _, path := range identicalFiles {
		for _, archive := range app.archives {
			delete(archive.files, path)
		}
	}
}

func (app *app) backupExcessFiles() {
	byHash := []map[string][]string{}
	for _, archive := range app.archives {
		hashMap := map[string][]string{}
		for _, file := range archive.files {
			if paths, ok := hashMap[file.hash]; ok {
				paths = append(paths, file.path)
				hashMap[file.hash] = paths
			} else {
				hashMap[file.hash] = []string{file.path}
			}
		}
		byHash = append(byHash, hashMap)
	}
	originalHashMap := byHash[0]
	for i, hashMap := range byHash[1:] {
		archive := app.archives[i+1]
		for hash, files := range hashMap {
			originalFiles := originalHashMap[hash]
			for len(files) > len(originalFiles) {
				path := files[len(files)-1]
				archive.commands = append(archive.commands, fs.Rename{
					SourcePath:      path,
					DestinationPath: filepath.Join(app.backup, path),
				})
				delete(archive.files, path)
				files = files[:len(files)-1]
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

func (app *app) renameFiles() {
	byHash := []map[string][]string{}
	for _, archive := range app.archives {
		hashMap := map[string][]string{}
		for _, file := range archive.files {
			if paths, ok := hashMap[file.hash]; ok {
				paths = append(paths, file.path)
				hashMap[file.hash] = paths
			} else {
				hashMap[file.hash] = []string{file.path}
			}
		}
		byHash = append(byHash, hashMap)
	}
	for hash, files := range byHash[0] {
		otherFiles := [][]string{}
		for _, hashMap := range byHash[1:] {
			otherFiles = append(otherFiles, hashMap[hash])
		}

	}
}

func (app *app) copyFiles() {
}

type appState int

const (
	appStarted appState = iota
	appAnalyzed
	appRunning
)

type archiveState int

const (
	archiveStarted archiveState = iota
	archiveScanned
	archiveHashed
)

type app struct {
	archives []*archive
	lc       *lifecycle.Lifecycle
	events   events
	backup   string
}

type archive struct {
	archiveState archiveState
	fs           fs.FS
	files        files
	commands     []any
	size         int
	done         int
}

type file struct {
	path    string
	size    int
	done    int
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
