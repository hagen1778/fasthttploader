package main

import (
	ui "github.com/gizak/termui"
)

type pBar struct {
	bar   *ui.Gauge
	num   int
	Grade uint64
}

func newBar(num int) *pBar {
	err := ui.Init()
	if err != nil {
		panic(err)
	}

	bar := ui.NewGauge()
	bar.Percent = 0
	bar.Width = 100
	bar.Height = 3
	bar.BorderLabel = "Progress"
	bar.BarColor = ui.ColorGreen
	bar.BorderFg = ui.ColorWhite
	bar.BorderLabelFg = ui.ColorCyan

	ui.Render(bar)

	return &pBar{
		bar: bar,
		num: num,
	}
}

func (b *pBar) inc() {
	b.bar.Percent = b.bar.Percent + 1
	b.Grade = uint64((b.bar.Percent * b.num) / 100)
	ui.Render(b.bar)
}

func (b *pBar) fin() {
	ui.Close()
}
