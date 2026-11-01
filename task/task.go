package task

import (
	"errors"
	"fmt"
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

type Generation int

func (g Generation) String() string {
	return []string{
		"Row", "Column", "Image", "Grid",
	}[g]
}

type Task struct {
	CurrentTask   uint
	ID            uint
	ImageNumber   uint
	Results       []Pixel
	Tasks         []Coordinate
	WorkerAddress string
}

func NewTask(id uint, imageNumber uint) Task {
	return Task{
		ID:          id,
		ImageNumber: imageNumber,
	}
}

func (t *Task) String() string {
	output := "{Task "
	output += fmt.Sprintf("ID: %d ", t.ID)
	output += fmt.Sprintf("Image Number: %d ", t.ImageNumber)
	output += fmt.Sprintf("Result Count: %d ", len(t.Results))
	output += fmt.Sprintf("Task Count: %d}", len(t.Tasks))
	return output
}

func (t *Task) AddTaskForPixel(coordinate Coordinate) {
	t.Tasks = append(t.Tasks, coordinate)
}

func (t *Task) AddTasksForRow(centerX float64, centerY float64, magnification float64, imageRow uint, imageWidth uint) {
	var c uint
	for c = 0; c < imageWidth; c++ {
		coordinate := Coordinate{
			CenterX:       centerX,
			CenterY:       centerY,
			Column:        c,
			Magnification: magnification,
			Row:           imageRow,
		}
		t.AddTaskForPixel(coordinate)
	}
}

func (t *Task) AddTasksForColumn(centerX float64, centerY float64, magnification float64, imageHeight uint, imageColumn uint) {
	var r uint
	for r = 0; r < imageHeight; r++ {
		coordinate := Coordinate{
			CenterX:       centerX,
			CenterY:       centerY,
			Column:        imageColumn,
			Magnification: magnification,
			Row:           r,
		}
		t.AddTaskForPixel(coordinate)
	}
}

func (t *Task) AddTasksForImage(centerX float64, centerY float64, magnification float64, imageHeight uint, imageWidth uint) {
	var r, c uint
	for r = 0; r < imageHeight; r++ {
		for c = 0; c < imageWidth; c++ {
			coordinate := Coordinate{
				CenterX:       centerX,
				CenterY:       centerY,
				Column:        c,
				Magnification: magnification,
				Row:           r,
			}
			t.AddTaskForPixel(coordinate)
		}
	}
}

// AddTasksForImageByGrid
// Adds the pixels for one cell of an image split into a GridDivisions x GridDivisions grid. gridRow and gridColumn are
// zero based. The last row and column of cells absorb any remainder so every pixel is covered exactly once.
func (t *Task) AddTasksForImageByGrid(centerX float64, centerY float64, magnification float64, imageHeight uint, imageWidth uint, gridRow uint, gridColumn uint) {
	cellHeight := imageHeight / GridDivisions
	cellWidth := imageWidth / GridDivisions
	rowStart, rowEnd := cellHeight*gridRow, cellHeight*(gridRow+1)
	columnStart, columnEnd := cellWidth*gridColumn, cellWidth*(gridColumn+1)
	if gridRow == GridDivisions-1 {
		rowEnd = imageHeight
	}
	if gridColumn == GridDivisions-1 {
		columnEnd = imageWidth
	}

	var r, c uint
	for r = rowStart; r < rowEnd; r++ {
		for c = columnStart; c < columnEnd; c++ {
			coordinate := Coordinate{
				CenterX:       centerX,
				CenterY:       centerY,
				Column:        c,
				Magnification: magnification,
				Row:           r,
			}
			t.AddTaskForPixel(coordinate)
		}
	}
}

// GetNextTask
// Returns the current task to be processed. Make sure to return the result to the AddResult method before calling
// this method again
func (t *Task) GetNextTask() (Coordinate, error) {
	if len(t.Results) >= len(t.Tasks) {
		return Coordinate{}, errors.New("no more tasks")
	}
	return t.Tasks[t.CurrentTask], nil
}

// AddResult
// When returning a result the CurrentTask value is incremented so the next call to the GetNextTask method will return
// the correct task
func (t *Task) AddResult(pixel Pixel) {
	t.Results = append(t.Results, pixel)
	t.CurrentTask++
}
