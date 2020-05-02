package main

type Task interface {
	NextTask() (row int, column int, magnification float64)
	RecordIteration(iteration int)
}

type TaskSettings struct {
	Boundary       float64
	CenterX        float64
	CenterY        float64
	Height         int
	MaxIterations  int
	SmoothColoring bool
	ShorterSide    int
	Width          int
}

type LineTask struct {
	currentWidth  int // current width value calculating
	ImageNumber   int
	Iterations    []float64
	Magnification float64
	Row           int
	Width         int // assumes 0 - width for column values
}

func (lt *LineTask) NextTask() (int, int, float64) {
	if len(lt.Iterations) < lt.Width {
		return lt.Row, lt.currentWidth, lt.Magnification
	}
	return -1, -1, -1
}
func (lt *LineTask) RecordIteration(iteration float64) {
	lt.Iterations = append(lt.Iterations, iteration)
	lt.currentWidth++
}
