package task

import (
	"fmt"
	"image"
)

const (
	Row Generation = iota
	Column
	Image
	Grid
)

// GridDivisions is the number of cells per side when using Grid task generation
const GridDivisions uint = 10

// NoMoreTasksMessage is the error the coordinator's GetTask returns once every task has been completed. RPC errors
// arrive at the worker as plain strings, so the worker compares against this exact message.
const NoMoreTasksMessage = "all tasks handed out"

// ProtocolVersion must match between the coordinator and its workers. gob silently ignores fields it does not
// recognize, so a mismatched pair would otherwise exchange tasks without error and produce blank images. Bump it
// whenever Task or another RPC payload changes shape.
const ProtocolVersion = 2

// BytesPerPixel is the size of one pixel in Task.Pixels: red, green, blue and alpha, the same layout as image.RGBA
const BytesPerPixel = 4

type Generation int

func (g Generation) String() string {
	return []string{
		"Row", "Column", "Image", "Grid",
	}[g]
}

// Regions splits a width x height frame into the rectangles that become one task each. Every pixel is covered by
// exactly one rectangle and no rectangle is empty. Returns nil for an unknown generation type.
func Regions(generation Generation, width uint, height uint) []image.Rectangle {
	w, h := int(width), int(height)
	var regions []image.Rectangle

	switch generation {
	case Row:
		for y := range h {
			regions = append(regions, image.Rect(0, y, w, y+1))
		}
	case Column:
		for x := range w {
			regions = append(regions, image.Rect(x, 0, x+1, h))
		}
	case Image:
		regions = append(regions, image.Rect(0, 0, w, h))
	case Grid:
		// The last row and column of cells absorb any remainder. In a frame smaller than the grid most cells are
		// empty, so those are skipped.
		divisions := int(GridDivisions)
		cellWidth, cellHeight := w/divisions, h/divisions
		for gridRow := range divisions {
			for gridColumn := range divisions {
				cell := image.Rect(gridColumn*cellWidth, gridRow*cellHeight, (gridColumn+1)*cellWidth, (gridRow+1)*cellHeight)
				if gridColumn == divisions-1 {
					cell.Max.X = w
				}
				if gridRow == divisions-1 {
					cell.Max.Y = h
				}
				if !cell.Empty() {
					regions = append(regions, cell)
				}
			}
		}
	}

	return regions
}

// Frame holds the parameters shared by every pixel of one image
type Frame struct {
	CenterX       float64
	CenterY       float64
	Magnification float64
}

// Registration is what a worker sends to the coordinator's RegisterWorker
type Registration struct {
	Address         string
	ProtocolVersion int
}

// Task is a rectangle of one image for a worker to render. The coordinator sends it without pixel data and the worker
// returns it with Pixels filled in.
type Task struct {
	Bounds        image.Rectangle // the region of the image this task covers
	Frame         Frame
	ID            uint
	ImageNumber   uint
	Pixels        []byte // packed RGBA, row by row within Bounds (the same layout as image.RGBA.Pix)
	WorkerAddress string
}

func NewTask(id uint, imageNumber uint, frame Frame, bounds image.Rectangle) Task {
	return Task{
		Bounds:      bounds,
		Frame:       frame,
		ID:          id,
		ImageNumber: imageNumber,
	}
}

// PixelCount is the number of pixels in the task's bounds
func (t *Task) PixelCount() int {
	return t.Bounds.Dx() * t.Bounds.Dy()
}

// ResultSize is the number of bytes Pixels must hold once the task is rendered
func (t *Task) ResultSize() int {
	return t.PixelCount() * BytesPerPixel
}

// HasValidResult reports whether Pixels holds exactly one color for every pixel in the task's bounds
func (t *Task) HasValidResult() bool {
	return len(t.Pixels) == t.ResultSize()
}

func (t *Task) String() string {
	return fmt.Sprintf("{Task ID: %d Image Number: %d Bounds: %v Pixel Bytes: %d}", t.ID, t.ImageNumber, t.Bounds, len(t.Pixels))
}
