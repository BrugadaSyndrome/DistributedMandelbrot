package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"net/rpc"
	"os"
	"os/exec"
	"sync"
	"time"
)

/* CoordinatorSettings */
type CoordinatorSettings struct {
	Boundary                float64
	EnableWebInterface      bool
	EscapeColor             color.RGBA
	GenerateMovie           bool
	GeneratePaletteSettings []GeneratePaletteSettings
	Height                  int
	MaxIterations           int
	RunName                 string
	Palette                 []color.RGBA
	SmoothColoring          bool
	SuperSampling           int
	TransitionSettings      []TransitionSettings
	Width                   int
}

type GeneratePaletteSettings struct {
	StartColor   color.RGBA
	EndColor     color.RGBA
	NumberColors int
}

type TransitionSettings struct {
	EndX               float64
	EndY               float64
	FrameCount         int
	MagnificationStart float64
	MagnificationEnd   float64
	MagnificationStep  float64
	StartX             float64
	StartY             float64
}

func (cs *CoordinatorSettings) GeneratePalette(settings []GeneratePaletteSettings) []color.RGBA {
	cs.Palette = make([]color.RGBA, 0)
	for i := 0; i < len(settings); i++ {
		for j := 0; j < settings[i].NumberColors; j++ {
			fraction := float64(j) / float64(settings[i].NumberColors)
			colorStep := color.RGBA{
				R: lerpUint8(settings[i].StartColor.R, settings[i].EndColor.R, fraction),
				G: lerpUint8(settings[i].StartColor.G, settings[i].EndColor.G, fraction),
				B: lerpUint8(settings[i].StartColor.B, settings[i].EndColor.B, fraction),
				A: 255}
			cs.Palette = append(cs.Palette, colorStep)
		}
	}
	return cs.Palette
}

func (cs *CoordinatorSettings) String() string {
	output := "\nCoordinator settings are: \n"
	output += fmt.Sprintf("Boundary: %f\n", cs.Boundary)
	output += fmt.Sprintf("Generate Movie: %t\n", cs.GenerateMovie)
	output += fmt.Sprintf("Generate Palette Settings: %v\n", cs.GeneratePaletteSettings)
	output += fmt.Sprintf("Height: %d\n", cs.Height)
	output += fmt.Sprintf("Enable Web Interface: %t\n", cs.EnableWebInterface)
	output += fmt.Sprintf("Escape Color: %v\n", cs.EscapeColor)
	output += fmt.Sprintf("Max Iterations: %d\n", cs.MaxIterations)
	output += fmt.Sprintf("Run Name: %s\n", cs.RunName)
	output += fmt.Sprintf("Palette: %v\n", cs.Palette)
	output += fmt.Sprintf("Smooth Coloring: %t\n", cs.SmoothColoring)
	output += fmt.Sprintf("Super Sampling: %d\n", cs.SuperSampling)
	output += fmt.Sprintf("Transition Settings: %v\n", cs.TransitionSettings)
	output += fmt.Sprintf("Width: %d\n", cs.Width)
	return output
}

func (cs *CoordinatorSettings) Verify() error {
	if cs.Boundary <= 0 {
		cs.Boundary = 100
	}
	// cs.EnableWebInterface defaults to false already
	if cs.EscapeColor == (color.RGBA{}) {
		cs.EscapeColor = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	if cs.Height <= 0 {
		cs.Height = 1080
	}
	// cs.GenerateMovie defaults to false already
	if len(cs.GeneratePaletteSettings) > 0 {
		cs.Palette = cs.GeneratePalette(cs.GeneratePaletteSettings)
	}
	if cs.MaxIterations <= 0 {
		cs.MaxIterations = 1000
	}
	if cs.RunName == "" {
		cs.RunName = "run_" + time.Now().Format("2006_01_02-03_04_05")
	}
	if len(cs.Palette) == 0 {
		cs.Palette = []color.RGBA{{R: 255, G: 255, B: 255, A: 255}}
	}
	// cs.SmoothColoring defaults to false already
	if cs.SuperSampling < 1 {
		cs.SuperSampling = 1
	}
	if len(cs.TransitionSettings) == 0 {
		cs.TransitionSettings = []TransitionSettings{
			{},
		}
	}
	for i := 0; i < len(cs.TransitionSettings); i++ {
		if cs.TransitionSettings[i].StartX < -4 || cs.TransitionSettings[i].StartX > 4 {
			cs.TransitionSettings[i].StartX = 0
		}
		if cs.TransitionSettings[i].StartY < -4 || cs.TransitionSettings[i].StartY > 4 {
			cs.TransitionSettings[i].StartY = 0
		}
		if cs.TransitionSettings[i].EndX < -4 || cs.TransitionSettings[i].EndX > 4 {
			cs.TransitionSettings[i].EndX = 0
		}
		if cs.TransitionSettings[i].EndY < -4 || cs.TransitionSettings[i].EndY > 4 {
			cs.TransitionSettings[i].EndY = 0
		}
		if cs.TransitionSettings[i].MagnificationEnd <= 0 {
			cs.TransitionSettings[i].MagnificationEnd = 1.5
		}
		if cs.TransitionSettings[i].MagnificationStart <= 0 {
			cs.TransitionSettings[i].MagnificationStart = 0.5
		}
		if cs.TransitionSettings[i].MagnificationStep <= 1 {
			cs.TransitionSettings[i].MagnificationStep = 1.1
		}
	}
	if cs.Width <= 0 {
		cs.Width = 1920
	}

	// Smooth coloring wont work with one color
	if len(cs.Palette) == 1 && cs.SmoothColoring == true {
		cs.SmoothColoring = false
		log.Printf("INFO - Disabling SmoothColoring since the palette only has one color.")
	}
	// If generate movie is set to true, verify ffmpeg is setup
	if cs.GenerateMovie {
		cmd := exec.Command("ffmpeg")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Run()
		if !bytes.Contains(stderr.Bytes(), []byte(`ffmpeg version`)) {
			cs.GenerateMovie = false
			log.Printf("INFO - Ffmpeg is not installed. Disabling GenerateMovie.")
		}
	}
	return nil
}

/* Coordinator */
type Coordinator struct {
	ImageCount     int
	ImageCompleted int
	ImageTasks     map[int]*ImageTask
	Logger         *log.Logger
	Mutex          sync.Mutex
	PixelCount     int
	Rectangle      image.Rectangle
	ShorterSide    int
	Settings       *CoordinatorSettings
	TaskCount      int
	TaskCompleted  int
	TasksDone      chan LineTask
	TaskSettings   *TaskSettings
	TasksTodo      chan LineTask
	Wait           *sync.WaitGroup
	Workers        map[string]*rpc.Client
}

type ImageTask struct {
	Image      *image.RGBA
	PixelsLeft int
}

func newCoordinator(settings CoordinatorSettings, ipAddress string, port int) Coordinator {
	shorterSide := settings.Height
	if settings.Width < settings.Height {
		shorterSide = settings.Width
	}

	/*
	 * Use logarithms to determine the number of images that will be generated using the specified magnification settings
	 * This basically reverses the exponential zooming that happens in the first for loop of the Coordinator.GenerateTasks method
	 *
	 * i.e.
	 * magnification_start + magnification_step^n = magnification_end
	 * log(magnification_start) + n = log_magnification_step(magnification_end)
	 * n = (log(magnification_end) / log(magnification_step)) - log(magnification_start)
	 *
	 */
	imageCount := 0
	for i := 0; i < len(settings.TransitionSettings); i++ {
		transitionCount := 0
		if settings.TransitionSettings[i].MagnificationStart < settings.TransitionSettings[i].MagnificationEnd {
			// zooming in
			transitionCount = int(math.Ceil((math.Log(settings.TransitionSettings[i].MagnificationEnd) / math.Log(settings.TransitionSettings[i].MagnificationStep)) - math.Log(settings.TransitionSettings[i].MagnificationStart)))
		} else {
			// zooming out
			transitionCount = int(math.Ceil((math.Log(settings.TransitionSettings[i].MagnificationStart) / math.Log(settings.TransitionSettings[i].MagnificationStep)) - math.Log(settings.TransitionSettings[i].MagnificationEnd)))
		}
		imageCount += transitionCount
		settings.TransitionSettings[i].FrameCount = transitionCount
	}

	coordinator := Coordinator{
		ImageCount: imageCount,
		ImageTasks: make(map[int]*ImageTask),
		Logger:     log.New(os.Stdout, fmt.Sprintf("Coordinator[%s:%d] ", ipAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		PixelCount: settings.Height * settings.Width,
		Rectangle: image.Rectangle{
			Min: image.Point{
				X: 0,
				Y: 0,
			},
			Max: image.Point{
				X: settings.Width,
				Y: settings.Height,
			},
		},
		Settings:    &settings,
		ShorterSide: shorterSide,
		TaskCount:   settings.Height * imageCount,
		TasksDone:   make(chan LineTask, 1000),
		TaskSettings: &TaskSettings{
			Boundary:           settings.Boundary,
			EscapeColor:        settings.EscapeColor,
			Height:             settings.Height,
			MaxIterations:      settings.MaxIterations,
			Palette:            settings.Palette,
			SmoothColoring:     settings.SmoothColoring,
			ShorterSide:        shorterSide,
			SuperSampling:      settings.SuperSampling,
			TransitionSettings: settings.TransitionSettings,
			Width:              settings.Width,
		},
		TasksTodo: make(chan LineTask, 1000),
		Wait:      &sync.WaitGroup{},
		Workers:   make(map[string]*rpc.Client, 0),
	}

	newRPCServer(&coordinator, ipAddress, port)

	return coordinator
}

func (c *Coordinator) callWorker(workerAddress string, method string, request interface{}, reply interface{}) error {
	err := c.Workers[workerAddress].Call(method, request, reply)

	// The call was a success
	if err == nil {
		return nil
	}

	c.Workers[workerAddress].Close()
	c.Logger.Printf("ERROR - Failed to call worker at address: %s, method: %s, error: %v", workerAddress, method, err)
	return err
}

func (c *Coordinator) heartBeat() {
	ticker := time.NewTicker(15 * time.Second)

	for {
		select {
		case _ = <-ticker.C:
			c.Logger.Printf("completed %d/%d images", c.ImageCompleted, c.ImageCount)
			c.Logger.Printf("completed %d/%d tasks", c.TaskCompleted, c.TaskCount)
		}
	}
}

func (c *Coordinator) GenerateTasks() {
	c.Logger.Printf("Generating %d tasks", c.TaskCount)

	imageNumber := 0
	// work through each transition
	for transitionStep := 0; transitionStep < len(c.Settings.TransitionSettings); transitionStep++ {

		// generate each image for this transition while zooming in exponentially
		transition := c.Settings.TransitionSettings[transitionStep]
		magnification := transition.MagnificationStart
		currentX := transition.StartX
		currentY := transition.StartY

		// generate each task for this image at this magnification
		for currentFrame := 1; currentFrame <= transition.FrameCount; currentFrame++ {

			if transition.MagnificationStart < transition.MagnificationEnd {
				// zooming in
				magnification *= transition.MagnificationStep
			} else {
				// zooming out
				magnification /= transition.MagnificationStep
			}

			for row := 0; row < c.Settings.Height; row++ {
				task := LineTask{
					CenterX:       currentX,
					CenterY:       currentY,
					CurrentWidth:  0,
					ImageNumber:   imageNumber,
					Colors:        make([]color.RGBA, 0),
					Magnification: magnification,
					Row:           row,
					Width:         c.Settings.Width,
				}

				c.Mutex.Lock()
				c.TasksTodo <- task
				c.Mutex.Unlock()
			}

			t := float64(currentFrame) / float64(transition.FrameCount)
			if transition.MagnificationStart < transition.MagnificationEnd {
				// zooming in
				currentX = lerpFloat64(transition.StartX, transition.EndX, easeOutExpo(t))
				currentY = lerpFloat64(transition.StartY, transition.EndY, easeOutExpo(t))
			} else {
				// zooming out
				currentX = lerpFloat64(transition.StartX, transition.EndX, easeInExpo(t))
				currentY = lerpFloat64(transition.StartY, transition.EndY, easeInExpo(t))
			}
			imageNumber++
		}

	}
	close(c.TasksTodo)

	c.Logger.Printf("Done generating %d tasks", c.TaskCount)
}

func (c *Coordinator) IngestTasks() {
	c.Logger.Print("Processing completed tasks")

	go c.heartBeat()

	digitCount := (int)(math.Log10((float64)(c.ImageCount)) + 1)
	for tc := 1; tc <= c.TaskCount; tc++ {
		task := <-c.TasksDone
		c.TaskCompleted++

		for it := 0; it < len(task.Colors); it++ {
			// Get the task
			imageTask, ok := c.ImageTasks[task.ImageNumber]

			// Create the image task if it does not exist
			if !ok {
				imageTask = &ImageTask{
					Image:      image.NewRGBA(c.Rectangle),
					PixelsLeft: c.PixelCount,
				}
			}

			// Draw the pixel on the image
			imageTask.Image.SetRGBA(it, task.Row, task.Colors[it])
			imageTask.PixelsLeft--

			// Save the task
			c.ImageTasks[task.ImageNumber] = imageTask

			// Generate the image once all pixels are filled
			if imageTask.PixelsLeft == 0 {
				path := fmt.Sprintf("%[1]s/%0[2]*[3]d.jpg", c.Settings.RunName, digitCount, task.ImageNumber)
				f, err := os.Create(path)
				if err != nil {
					c.Logger.Fatalf("ERROR - Unable to create image: %s", err)
				}
				err = jpeg.Encode(f, imageTask.Image, nil)
				if err != nil {
					c.Logger.Fatalf("ERROR - Unable to save image: %s", err)
				}
				// Remove the image to conserve memory
				c.Mutex.Lock()
				delete(c.ImageTasks, task.ImageNumber)
				c.ImageCompleted++
				c.Mutex.Unlock()

				c.Logger.Printf("Saved image to ./%s", path)
			}
		}
	}

	c.Logger.Print("Done processing completed tasks")
}

func (c *Coordinator) RequestTask(request Nothing, reply *LineTask) error {
	c.Mutex.Lock()
	task, more := <-c.TasksTodo
	c.Mutex.Unlock()
	if !more {
		reply = nil
		c.Logger.Print("Telling worker all tasks are handed out")
		return errors.New("all tasks handed out")
	}
	*reply = task
	return nil
}

func (c *Coordinator) TaskFinished(request LineTask, reply *Nothing) error {
	c.Mutex.Lock()
	c.TasksDone <- request
	c.Mutex.Unlock()
	return nil
}

func (c *Coordinator) GetTaskSettings(request Nothing, reply *TaskSettings) error {
	c.Mutex.Lock()

	// todo: figure out why I cannot just assign the c.TaskSettings struct to reply...
	// reply = c.TaskSettings

	reply.Boundary = c.Settings.Boundary
	reply.EscapeColor = c.Settings.EscapeColor
	reply.Height = c.Settings.Height
	reply.MaxIterations = c.Settings.MaxIterations
	reply.Palette = c.Settings.Palette
	reply.ShorterSide = c.ShorterSide
	reply.SmoothColoring = c.Settings.SmoothColoring
	reply.SuperSampling = c.Settings.SuperSampling
	reply.TransitionSettings = c.Settings.TransitionSettings
	reply.Width = c.Settings.Width
	c.Mutex.Unlock()
	return nil
}

func (c *Coordinator) RegisterWorker(request string, reply *Nothing) error {
	client, err := rpc.DialHTTP("tcp", request)
	if err != nil {
		c.Logger.Fatalf("Failed registering worker at address: %s - %s", request, err)
	}
	c.Logger.Printf("Opened connection to worker at %s", request)

	c.Mutex.Lock()
	c.Workers[request] = client
	c.Wait.Add(1)
	c.Mutex.Unlock()
	return nil
}

func (c *Coordinator) DeRegisterWorker(request string, reply *Nothing) error {
	err := c.Workers[request].Close()
	if err != nil {
		c.Logger.Fatalf("Failed de-registering worker at address: %s - %s", request, err)
	}
	c.Logger.Printf("Closed connection to worker at %s", request)

	c.Mutex.Lock()
	c.Wait.Done()
	c.Mutex.Unlock()
	return nil
}
