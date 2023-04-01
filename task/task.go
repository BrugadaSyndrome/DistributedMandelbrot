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

type Generation int

func (g Generation) String() string {
	return []string{
		"Row", "Column", "Image",
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

func (t *Task) AddTasksForImageByGrid(centerX float64, centerY float64, magnification float64, imageHeight uint, imageWidth uint, percentage uint, gridRow uint, gridColumn uint) {
	var r, c uint
	for r = (imageHeight / percentage) * gridRow; r < imageHeight/percentage; r++ {
		for c = (imageWidth / percentage) * gridColumn; c < imageWidth/percentage; c++ {
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
