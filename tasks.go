package main

import "image/color"

type Task interface {
	NextTask() (row int, column int, magnification float64)
	RecordColor(color color.RGBA)
}

type TaskSettings struct {
	Boundary       float64
	CenterX        float64
	CenterY        float64
	Colors         []color.RGBA
	Height         int
	MaxIterations  int
	SmoothColoring bool
	ShorterSide    int
	SuperSampling  int
	Width          int
}

type LineTask struct {
	currentWidth  int // current width value calculating
	ImageNumber   int
	Colors        []color.RGBA
	Magnification float64
	Row           int
	Width         int // assumes 0 - width for column values
}

func (lt *LineTask) NextTask() (int, int, float64) {
	if len(lt.Colors) < lt.Width {
		return lt.Row, lt.currentWidth, lt.Magnification
	}
	return -1, -1, -1
}

func (lt *LineTask) RecordColor(color color.RGBA) {
	lt.Colors = append(lt.Colors, color)
	lt.currentWidth++
}
