package main

/*
	different methods may include pixel, row, chunk
	pixel: each task is to calculate individual pixels of the image
	row: each task is to calculate an entire row of the image
	chunk: each task is a square area of the image (most useful once we want to AA the image)
	image: each task is to generate one frame of the image (probably only useful if many, many images are being generated)
*/

type Task interface {
	NextTask() (row int, column int, magnification float64)
	RecordIteration(iteration int)
}

type TaskSettings struct {
	Boundary      float64
	CenterX       float64
	CenterY       float64
	Height        int
	MaxIterations int
	ShorterSide   int
	Width         int
}

// best case O(R) or O(C); normal case O(R*C); worst case O(R^2) or O(C^2) tasks per image
// Cons: will not work well when doing AA without adding another phase just for that???
type PixelTask struct {
	Row           int
	Column        int
	ImageNumber   int
	Iterations    int
	Magnification float64
}

func (pt *PixelTask) NextTask() (int, int, float64) {
	if pt.Iterations != 0 {
		return -1, -1, -1
	}
	return pt.Row, pt.Column, pt.Magnification
}
func (pt *PixelTask) RecordIteration(iteration int) {
	if pt.Iterations == 0 {
		pt.Iterations = iteration
	}
}

// best/normal/worst case O(R) tasks per image
// Cons: Still will not work well with AA without post processing or something
type LineTask struct {
	currentWidth  int // current width value calculating
	ImageNumber   int
	Iterations    []int
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
func (lt *LineTask) RecordIteration(iteration int) {
	lt.Iterations = append(lt.Iterations, iteration)
	lt.currentWidth++
}

// best/normal/worst case O(1) tasks per image
// Cons: Probably will not balance out well enough between workers...
// Pros: Will be easy to AA
type imageTask struct {
	Height int
	Width  int
}

// todo: analyze this method
type chunkTask struct {
	StartRow    int
	EndRow      int
	StartColumn int
	EndColumn   int
}
