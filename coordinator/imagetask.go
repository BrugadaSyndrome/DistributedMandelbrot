package coordinator

import "image"

type imageTask struct {
	Image      *image.RGBA
	PixelsLeft uint
}
