package main

/*
type task interface {
}

type taskFactory struct {
}

func newTaskFactory(taskType string) task {
	return nil
}
*/

/*
	different methods may include pixel, row, chunk
	pixel: each task is to calculate individual pixels of the image
	row: each task is to calculate an entire row of the image
	chunk: each task is a square area of the image (most useful once we want to AA the image)
	image: each task is to generate one frame of the image (probably only useful if many, many images are being generated)
*/

// best case O(R) or O(C); normal case O(R*C); worst case O(R^2) or O(C^2) tasks per image
// Cons: will not work well when doing AA without adding another phase just for that???
type pixelTask struct {
	Row           int
	Column        int
	Magnification float64
}

// best/normal/worst case O(R) tasks per image
// Cons: Still will not work well with AA without post processing or something
type rowTask struct {
	Row           int
	Width         int // assumes 0 - width for column values
	Magnification float64
}
type columnTask struct {
	Height        int // assumes 0 - height for row values
	Column        int
	Magnification float64
}

// best/normal/worst case O(1) tasks per image
// Cons: Probably will not balance out well enough between workers...
// Pros: Will be easy to AA
type imageTask struct {
	Height        int
	Width         int
	Magnification float64
}

// todo: analyze this method
type chunkTask struct {
	StartRow    int
	EndRow      int
	StartColumn int
	EndColumn   int
}
