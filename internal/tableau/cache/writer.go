package cache

import (
	"context"
	"sync"
)

type batchPump struct {
	ctx    context.Context
	cancel context.CancelFunc
	writer BatchWriter
	queue  chan Batch
	done   chan struct{}
	once   sync.Once
	errMu  sync.Mutex
	err    error
}

func newBatchPump(ctx context.Context, cancel context.CancelFunc, writer BatchWriter, queueSize int) *batchPump {
	pump := &batchPump{ctx: ctx, cancel: cancel, writer: writer, queue: make(chan Batch, queueSize), done: make(chan struct{})}
	go pump.run()
	return pump
}

func (p *batchPump) run() {
	defer close(p.done)
	for {
		select {
		case <-p.ctx.Done():
			return
		case batch, ok := <-p.queue:
			if !ok {
				return
			}
			if err := p.writer.WriteBatch(p.ctx, batch); err != nil {
				p.errMu.Lock()
				p.err = err
				p.errMu.Unlock()
				p.cancel()
				return
			}
		}
	}
}

func (p *batchPump) Emit(ctx context.Context, batch Batch) error {
	select {
	case <-ctx.Done():
		if err := p.Err(); err != nil {
			return err
		}
		return ctx.Err()
	case <-p.done:
		if err := p.Err(); err != nil {
			return err
		}
		return p.ctx.Err()
	case p.queue <- batch:
		return nil
	}
}

func (p *batchPump) Close() error {
	p.once.Do(func() { close(p.queue) })
	<-p.done
	return p.Err()
}

func (p *batchPump) Err() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}
