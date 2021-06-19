package task

import (
	"fmt"
	"image/color"
)

type Pixel struct {
	Color  color.RGBA
	Column uint
	Row    uint
}

func (p *Pixel) String() string {
	output := "{Pixel "
	output += fmt.Sprintf("Color: %v ", p.Color)
	output += fmt.Sprintf("Column: %d ", p.Column)
	output += fmt.Sprintf("Row: %d}", p.Row)
	return output
}
