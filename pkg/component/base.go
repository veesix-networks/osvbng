package component

import (
	"context"
	"sync"
)

type Base struct {
	name   string
	Ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewBase(name string) *Base {
	return &Base{name: name}
}

func (b *Base) Name() string {
	return b.name
}

func (b *Base) StartContext(parentCtx context.Context) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	b.Ctx, b.cancel = context.WithCancel(parentCtx)
}

func (b *Base) StopContext() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
}

func (b *Base) Go(fn func()) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		fn()
	}()
}
