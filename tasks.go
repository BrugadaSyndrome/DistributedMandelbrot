package main

import "image/color"

type Task interface {
	NextTask() (row int, column int, magnification float64)
	RecordColor(color color.RGBA)
}

type TaskSettings struct {
	Boundary           float64
	CenterX            float64
	CenterY            float64
	EscapeColor        color.RGBA
	Height             int
	MaxIterations      int
	Palette            []color.RGBA
	SmoothColoring     bool
	ShorterSide        int
	SuperSampling      int
	TransitionSettings []TransitionSettings
	Width              int
}

type LineTask struct {
	CenterX       float64
	CenterY       float64
	CurrentWidth  int // current width value calculating
	ImageNumber   int
	Colors        []color.RGBA
	Magnification float64
	Row           int
	Width         int // assumes 0 - width for column values
}

func (lt *LineTask) NextTask() (int, int, float64, float64, float64) {
	if len(lt.Colors) < lt.Width {
		return lt.Row, lt.CurrentWidth, lt.Magnification, lt.CenterX, lt.CenterY
	}
	return -1, -1, -1, -1, -1
}

func (lt *LineTask) RecordColor(color color.RGBA) {
	lt.Colors = append(lt.Colors, color)
	lt.CurrentWidth++
}
