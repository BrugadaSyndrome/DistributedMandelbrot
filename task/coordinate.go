package task

import "fmt"

type Coordinate struct {
	CenterX       float64
	CenterY       float64
	Column        uint
	Magnification float64
	Row           uint
}

func (c *Coordinate) String() string {
	output := "{Coordinate "
	output += fmt.Sprintf("CenterX: %f ", c.CenterX)
	output += fmt.Sprintf("CenterY: %f ", c.CenterY)
	output += fmt.Sprintf("Column: %d ", c.Column)
	output += fmt.Sprintf("Magnification: %f ", c.Magnification)
	output += fmt.Sprintf("Row: %d}", c.Row)
	return output
}
