package app

import (
	"fmt"
	"log"
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
		archives[i] = &archive{fs: fs}
	}

	state := &state{archives: archives, lc: lc, events: events{p}}
	m <- state

	for _, fs := range fss {
		fs.Scan(state.events)
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

type model chan *state

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case fs.FileMetas:
		state := <-m
		archive := state.archives[msg.Idx]
		for _, meta := range msg.Metas {
			archive.files = append(archive.files, &file{
				path:    meta.Path,
				size:    meta.Size,
				modTime: meta.ModTime,
				hash:    meta.Hash,
			})
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
	}
	return m, nil
}

func (m model) View() string {
	state := <-m
	b := strings.Builder{}
	for _, archive := range state.archives {
		switch archive.archiveState {
		case archiveStarted:
			fmt.Fprintf(&b, "scanning %s\n", archive.fs.Root())
		case archiveScanned:
			var done float64 = 100
			if archive.size > 0 {
				done = float64(archive.done) * 100 / float64(archive.size)
			}
			fmt.Fprintf(&b, "hashing %5.2f: %s\n", done, archive.fs.Root())
		}
	}
	m <- state
	return b.String()
}

type archiveState int

const (
	archiveStarted archiveState = iota
	archiveScanned
	archiveHashed
)

type state struct {
	archives []*archive
	lc       *lifecycle.Lifecycle
	events   events
}

type archive struct {
	archiveState archiveState
	fs           fs.FS
	files        files
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

type files []*file

func (arc *archive) findFile(path string) *file {
	for _, file := range arc.files {
		if file.path == path {
			return file
		}
	}
	return nil
}
