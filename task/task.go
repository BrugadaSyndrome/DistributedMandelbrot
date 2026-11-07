package task

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
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
const ProtocolVersion = 3

// BytesPerPixel is the size of one pixel in Task.Pixels: red, green, blue and alpha, the same layout as image.RGBA
const BytesPerPixel = 4

// BytesPerSample is the size of one sample in the Pixels of a tile task: a little endian float32
const BytesPerSample = 4

// TileSize is the width and height in pixels of an explorer tile
const TileSize = 256

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

// Tile holds what a worker needs to compute one explorer tile. The explorer colors tiles in the browser, so a tile task
// returns smooth iteration counts instead of colors and carries only the settings that change those counts.
type Tile struct {
	Boundary      float64
	MaxIterations uint
	OriginX       float64 // complex coordinate of the tile's top left corner
	OriginY       float64
	PixelSize     float64 // width and height of one pixel in complex units
	SuperSampling int     // samples per pixel along each side
}

// Task is a rectangle of one image for a worker to render. The coordinator sends it without pixel data and the worker
// returns it with Pixels filled in.
type Task struct {
	Bounds      image.Rectangle // the region of the image this task covers
	Frame       Frame
	ID          uint
	ImageNumber uint
	// Pixels is packed RGBA, row by row within Bounds (the same layout as image.RGBA.Pix). For a tile task it is
	// SuperSampling² samples per pixel instead, each encoded with EncodeSamples.
	Pixels        []byte
	Tile          *Tile // set for explorer tiles, nil for movie frames
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
	if t.Tile != nil {
		return t.PixelCount() * t.Tile.SuperSampling * t.Tile.SuperSampling * BytesPerSample
	}
	return t.PixelCount() * BytesPerPixel
}

// EncodeSamples packs samples as little endian float32 values, the format of a tile task's Pixels
func EncodeSamples(samples []float32) []byte {
	encoded := make([]byte, 0, len(samples)*BytesPerSample)
	for _, sample := range samples {
		encoded = binary.LittleEndian.AppendUint32(encoded, math.Float32bits(sample))
	}
	return encoded
}

// DecodeSamples is the reverse of EncodeSamples. A trailing partial sample is ignored.
func DecodeSamples(encoded []byte) []float32 {
	samples := make([]float32, len(encoded)/BytesPerSample)
	for i := range samples {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[i*BytesPerSample:]))
	}
	return samples
}

// HasValidResult reports whether Pixels holds exactly one color for every pixel in the task's bounds
func (t *Task) HasValidResult() bool {
	return len(t.Pixels) == t.ResultSize()
}

func (t *Task) String() string {
	return fmt.Sprintf("{Task ID: %d Image Number: %d Bounds: %v Pixel Bytes: %d}", t.ID, t.ImageNumber, t.Bounds, len(t.Pixels))
}
