//go:build !headless

package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const statusBadgeLabelNudgeY = float32(0)
const statusBadgeLabelGapX = float32(0)

type statusBadgeLabelOptions struct {
	DotNudgeY float32
	GapX      float32
}

type statusBadgeLabel struct {
	object *fyne.Container
	badge  *statusBadge
	label  *widget.Label
}

func newStatusBadgeLabel(handlers statusBadgeHandlers, text string, opts statusBadgeLabelOptions) *statusBadgeLabel {
	badge := newStatusBadge(handlers)
	badge.SetCompact(true)
	nudge := opts.DotNudgeY
	if nudge == 0 {
		nudge = statusBadgeLabelNudgeY
	}
	badge.SetDotNudgeY(nudge)
	label := widget.NewLabel(text)
	gapX := opts.GapX
	if gapX < 0 {
		gapX = 0
	}
	if gapX == 0 {
		gapX = statusBadgeLabelGapX
	}
	leftItems := []fyne.CanvasObject{badge}
	if gapX > 0 {
		gap := canvas.NewRectangle(color.Transparent)
		gap.SetMinSize(fyne.NewSize(gapX, 1))
		leftItems = append(leftItems, gap)
	}
	left := container.NewHBox(leftItems...)
	object := container.NewBorder(nil, nil, left, nil, label)
	return &statusBadgeLabel{
		object: object,
		badge:  badge,
		label:  label,
	}
}

func (s *statusBadgeLabel) Object() fyne.CanvasObject {
	return s.object
}

func (s *statusBadgeLabel) Label() *widget.Label {
	return s.label
}

func (s *statusBadgeLabel) SetText(text string) {
	s.label.SetText(text)
}

func (s *statusBadgeLabel) SetStatus(fill color.NRGBA, tooltip string) {
	s.badge.SetStatus(fill, tooltip)
}
