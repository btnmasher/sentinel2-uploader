//go:build !headless

package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	sliderToggleWidth  = float32(44)
	sliderToggleHeight = float32(24)
)

type sliderToggle struct {
	widget.BaseWidget

	Checked   bool
	OnChanged func(bool)

	track *canvas.Rectangle
	thumb *canvas.Circle
}

func newSliderToggle(onChanged func(bool)) *sliderToggle {
	t := &sliderToggle{
		OnChanged: onChanged,
		track:     canvas.NewRectangle(color.NRGBA{R: 120, G: 120, B: 120, A: 255}),
		thumb:     canvas.NewCircle(color.NRGBA{R: 245, G: 245, B: 245, A: 255}),
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *sliderToggle) SetChecked(checked bool) {
	if t.Checked == checked {
		return
	}
	t.Checked = checked
	if t.OnChanged != nil {
		t.OnChanged(checked)
	}
	t.Refresh()
}

func (t *sliderToggle) MinSize() fyne.Size {
	return fyne.NewSize(sliderToggleWidth, sliderToggleHeight)
}

func (t *sliderToggle) Tapped(*fyne.PointEvent) {
	t.SetChecked(!t.Checked)
}

func (t *sliderToggle) TappedSecondary(*fyne.PointEvent) {}

func (t *sliderToggle) CreateRenderer() fyne.WidgetRenderer {
	return &sliderToggleRenderer{toggle: t, objs: []fyne.CanvasObject{t.track, t.thumb}}
}

type sliderToggleRenderer struct {
	toggle *sliderToggle
	objs   []fyne.CanvasObject
}

func (r *sliderToggleRenderer) Layout(size fyne.Size) {
	height := size.Height
	if height > sliderToggleHeight {
		height = sliderToggleHeight
	}
	if height < 16 {
		height = 16
	}

	width := size.Width
	if width < sliderToggleWidth {
		width = sliderToggleWidth
	}

	radius := height / 2
	r.toggle.track.CornerRadius = radius
	r.toggle.track.Resize(fyne.NewSize(width, height))
	r.toggle.track.Move(fyne.NewPos(0, 0))

	thumbDiameter := height - 4
	if thumbDiameter < 10 {
		thumbDiameter = 10
	}
	thumbY := (height - thumbDiameter) / 2
	thumbX := float32(2)
	if r.toggle.Checked {
		thumbX = width - thumbDiameter - 2
	}
	r.toggle.thumb.Resize(fyne.NewSize(thumbDiameter, thumbDiameter))
	r.toggle.thumb.Move(fyne.NewPos(thumbX, thumbY))
}

func (r *sliderToggleRenderer) MinSize() fyne.Size {
	return fyne.NewSize(sliderToggleWidth, sliderToggleHeight)
}

func (r *sliderToggleRenderer) Refresh() {
	r.Layout(r.toggle.Size())
	if r.toggle.Checked {
		r.toggle.track.FillColor = theme.Color(theme.ColorNamePrimary)
	} else {
		r.toggle.track.FillColor = color.NRGBA{R: 115, G: 115, B: 115, A: 255}
	}
	r.toggle.thumb.FillColor = theme.Color(theme.ColorNameForeground)
	canvas.Refresh(r.toggle.track)
	canvas.Refresh(r.toggle.thumb)
}

func (r *sliderToggleRenderer) Objects() []fyne.CanvasObject {
	return r.objs
}

func (r *sliderToggleRenderer) Destroy() {}
