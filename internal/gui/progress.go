package gui

import (
	"context"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// progressEmitter sends progress updates to the frontend.
// It is nil-safe: calling methods on a nil pointer is a no-op.
type progressEmitter struct {
	ctx     context.Context
	context string
}

func newProgressEmitter(ctx context.Context, context string) *progressEmitter {
	if ctx == nil {
		return nil
	}
	return &progressEmitter{
		ctx:     ctx,
		context: context,
	}
}

func (p *progressEmitter) update(current, total int) {
	if p == nil || total <= 0 {
		return
	}
	p.emit(current, total)
}

func (p *progressEmitter) emit(current, total int) {
	if p == nil {
		return
	}
	wruntime.EventsEmit(p.ctx, "progress", map[string]any{
		"context": p.context,
		"current": current,
		"total":   total,
	})
}
