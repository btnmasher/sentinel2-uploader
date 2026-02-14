//go:build !headless

package gui

import (
	"image/color"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	badgeDotSize      = float32(12)
	badgeHoverTarget  = float32(24)
	badgeHoverSlack   = float32(3)
	badgeTooltipDelay = 180 * time.Millisecond
)

type statusBadge struct {
	widget.BaseWidget

	fill    color.NRGBA
	tooltip string

	dot *canvas.Circle

	onTooltipShow func(string, fyne.Position)
	onTooltipMove func(fyne.Position)
	onTooltipHide func()

	hoverTimer  *time.Timer
	hideTimer   *time.Timer
	hoverSeq    atomic.Uint64
	shown       bool
	hovered     bool
	hoverPos    fyne.Position
	hasHoverPos bool
}

var _ desktop.Hoverable = (*statusBadge)(nil)

type statusBadgeHandlers struct {
	Show func(string, fyne.Position)
	Move func(fyne.Position)
	Hide func()
}

func newStatusBadge(handlers statusBadgeHandlers) *statusBadge {
	b := &statusBadge{
		fill:          channelRedColor,
		onTooltipShow: handlers.Show,
		onTooltipMove: handlers.Move,
		onTooltipHide: handlers.Hide,
	}
	b.dot = canvas.NewCircle(b.fill)
	b.ExtendBaseWidget(b)
	return b
}

func (b *statusBadge) SetStatus(fill color.NRGBA, tooltip string) {
	b.fill = fill
	b.tooltip = tooltip
	b.dot.FillColor = fill
	b.dot.Refresh()
	if tooltip == "" {
		b.hideTooltip()
		return
	}
	if b.shown {
		b.showTooltipNow()
	}
}

func (b *statusBadge) MinSize() fyne.Size {
	text := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{})
	height := text.Height
	if height < badgeDotSize+2 {
		height = badgeDotSize + 2
	}
	height = max(height, badgeHoverTarget)
	return fyne.NewSize(badgeHoverTarget, height)
}

func (b *statusBadge) CreateRenderer() fyne.WidgetRenderer {
	anchor := canvas.NewRectangle(color.Transparent)
	anchor.SetMinSize(b.MinSize())
	dot := container.NewGridWrap(fyne.NewSize(badgeDotSize, badgeDotSize), b.dot)
	wrapped := container.NewStack(anchor, container.NewCenter(dot))
	return widget.NewSimpleRenderer(wrapped)
}

func (b *statusBadge) MouseIn(ev *desktop.MouseEvent) {
	b.hovered = true
	b.cancelHideTimer()
	if ev != nil {
		b.hoverPos = b.pointerCanvasPos(ev.Position)
		b.hasHoverPos = true
	}
	b.scheduleTooltip()
}

func (b *statusBadge) MouseMoved(ev *desktop.MouseEvent) {
	b.hovered = true
	b.cancelHideTimer()
	if ev != nil {
		b.hoverPos = b.pointerCanvasPos(ev.Position)
		b.hasHoverPos = true
	}
	if b.shown {
		b.moveTooltip()
		return
	}
	b.scheduleTooltip()
}

func (b *statusBadge) MouseOut() {
	b.hovered = false
	b.cancelTooltipTimer()
	b.scheduleHideTooltip()
}

func (b *statusBadge) scheduleTooltip() {
	if b.tooltip == "" || b.shown || b.hoverTimer != nil {
		return
	}
	seq := b.hoverSeq.Add(1)
	b.hoverTimer = time.AfterFunc(badgeTooltipDelay, func() {
		fyne.Do(func() {
			b.hoverTimer = nil
			if b.hoverSeq.Load() != seq {
				return
			}
			b.showTooltipNow()
		})
	})
}

func (b *statusBadge) cancelTooltipTimer() {
	b.hoverSeq.Add(1)
	if b.hoverTimer != nil {
		b.hoverTimer.Stop()
		b.hoverTimer = nil
	}
}

func (b *statusBadge) scheduleHideTooltip() {
	b.cancelHideTimer()
	b.hideTimer = time.AfterFunc(120*time.Millisecond, func() {
		fyne.Do(func() {
			b.hideTimer = nil
			if b.hovered {
				return
			}
			b.hideTooltip()
			b.hasHoverPos = false
		})
	})
}

func (b *statusBadge) cancelHideTimer() {
	if b.hideTimer != nil {
		b.hideTimer.Stop()
		b.hideTimer = nil
	}
}

func (b *statusBadge) showTooltipNow() {
	if b.tooltip == "" {
		b.hideTooltip()
		return
	}
	anchor := b.tooltipAnchor()
	if b.onTooltipShow != nil {
		b.onTooltipShow(b.tooltip, anchor)
	}
	b.shown = true
}

func (b *statusBadge) moveTooltip() {
	if !b.shown {
		return
	}
	if b.onTooltipMove != nil {
		b.onTooltipMove(b.tooltipAnchor())
	}
}

func (b *statusBadge) tooltipAnchor() fyne.Position {
	anchor := fyne.NewPos(0, 0)
	if b.hasHoverPos {
		anchor = b.hoverPos
	} else if app := fyne.CurrentApp(); app != nil {
		pos := app.Driver().AbsolutePositionForObject(b)
		if pos == (fyne.Position{}) {
			pos = app.Driver().AbsolutePositionForObject(b.dot)
		}
		target := b.Size()
		anchor = fyne.NewPos(pos.X+target.Width, pos.Y+target.Height/2)
	}
	return anchor
}

func (b *statusBadge) pointerCanvasPos(local fyne.Position) fyne.Position {
	app := fyne.CurrentApp()
	if app == nil {
		return local
	}
	base := app.Driver().AbsolutePositionForObject(b)
	return fyne.NewPos(base.X+local.X, base.Y+local.Y)
}

func (b *statusBadge) hideTooltip() {
	if b.onTooltipHide != nil {
		b.onTooltipHide()
	}
	b.shown = false
}
