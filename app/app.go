package app

import (
	"fmt"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"dup/fs"
	"dup/lifecycle"
)

func Run(fss []fs.FS, lc *lifecycle.Lifecycle) {
	m := make(model, 1)
	m <- &state{fss: fss, lc: lc}
	p := tea.NewProgram(m)
	for _, fs := range fss {
		go fs.Run()
	}

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

type model chan *state

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case int:
		state := <-m
		state.i++
		m <- state
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	state := <-m
	result := fmt.Sprintf("Hi: %d\n", state.i)
	m <- state
	return result
}

type state struct {
	i   int
	fss []fs.FS
	lc  *lifecycle.Lifecycle
}
