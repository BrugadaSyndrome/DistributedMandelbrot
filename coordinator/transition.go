package coordinator

type transitionSettings struct {
	EndX               float64
	EndY               float64
	FrameCount         uint
	MagnificationStart float64
	MagnificationEnd   float64
	MagnificationStep  float64
	StartX             float64
	StartY             float64
}

func (ts *transitionSettings) Verify() error {
	if ts.StartX < -4 || ts.StartX > 4 {
		ts.StartX = 0
	}
	if ts.StartY < -4 || ts.StartY > 4 {
		ts.StartY = 0
	}
	if ts.EndX < -4 || ts.EndX > 4 {
		ts.EndX = 0
	}
	if ts.EndY < -4 || ts.EndY > 4 {
		ts.EndY = 0
	}
	if ts.MagnificationEnd <= 0 {
		ts.MagnificationEnd = 1.5
	}
	if ts.MagnificationStart <= 0 {
		ts.MagnificationStart = 0.5
	}
	if ts.MagnificationStep <= 1 {
		ts.MagnificationStep = 1.1
	}
	return nil
}
