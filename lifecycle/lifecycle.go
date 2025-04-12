package lifecycle

import (
	"context"
	"sync"
)

type Lifecycle struct {
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

func New() *Lifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifecycle{wg: sync.WaitGroup{}, ctx: ctx, cancel: cancel}
}

func (lc *Lifecycle) Started() {
	lc.wg.Add(1)
}

func (lc *Lifecycle) Done() {
	lc.wg.Done()
}

func (lc *Lifecycle) ShoudStop() bool {
	select {
	case <-lc.ctx.Done():
		return true
	default:
		return false
	}
}

func (lc *Lifecycle) Stop() {
	lc.cancel()
	lc.wg.Wait()
}
