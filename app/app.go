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

	scanStates := make([]scanState, len(fss))
	state := &state{fss: fss, lc: lc, events: events{p}, scanStates: scanStates}

	for _, fs := range fss {
		fs.Scan(state.events)
	}

	m <- state

	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second)
			p.Send(i)
		}
	}()

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

	case fs.EventDebugScan:
		state := <-m
		state.scanStates[msg.Idx].n = msg.N
		m <- state
	}
	return m, nil
}

func (m model) View() string {
	state := <-m
	b := strings.Builder{}
	for i, scanState := range state.scanStates {
		fmt.Fprintf(&b, "scanning %d: %d\n", i, scanState.n)
	}
	m <- state
	return b.String()
}

type state struct {
	fss        []fs.FS
	lc         *lifecycle.Lifecycle
	events     events
	scanStates []scanState
}

type scanState struct {
	n int
}
