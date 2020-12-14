package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

/* CoordinatorSettings */
type CoordinatorSettings struct {
	Boundary                float64
	CenterX                 float64
	CenterY                 float64
	EnableWebInterface      bool
	EscapeColor             color.RGBA
	GeneratePaletteSettings []GeneratePaletteSettings
	Height                  int
	MagnificationEnd        float64
	MagnificationStart      float64
	MagnificationStep       float64
	MaxIterations           int
	Palette                 []color.RGBA
	SmoothColoring          bool
	SuperSampling           int
	Width                   int
}

type GeneratePaletteSettings struct {
	StartColor   color.RGBA
	EndColor     color.RGBA
	NumberColors int
}

func (cs *CoordinatorSettings) GeneratePalette(settings []GeneratePaletteSettings) []color.RGBA {
	cs.Palette = make([]color.RGBA, 0)
	for i := 0; i < len(settings); i++ {
		for j := 0; j < settings[i].NumberColors; j++ {
			fraction := float64(j) / float64(settings[i].NumberColors)
			colorStep := color.RGBA{
				R: interpolateUint8(settings[i].StartColor.R, settings[i].EndColor.R, fraction),
				G: interpolateUint8(settings[i].StartColor.G, settings[i].EndColor.G, fraction),
				B: interpolateUint8(settings[i].StartColor.B, settings[i].EndColor.B, fraction),
				A: 255}
			cs.Palette = append(cs.Palette, colorStep)
		}
	}
	log.Printf("%+v", cs.Palette)
	return cs.Palette
}

func (cs *CoordinatorSettings) String() string {
	output := "\nCoordinator settings are: \n"
	output += fmt.Sprintf("Boundary: %f\n", cs.Boundary)
	output += fmt.Sprintf("CenterX: %f\n", cs.CenterX)
	output += fmt.Sprintf("CenterY: %f\n", cs.CenterY)
	output += fmt.Sprintf("Generate Palette Settings: %v\n", cs.GeneratePaletteSettings)
	output += fmt.Sprintf("Height: %d\n", cs.Height)
	output += fmt.Sprintf("Enable Web Interface: %t\n", cs.EnableWebInterface)
	output += fmt.Sprintf("Escape Color: %v\n", cs.EscapeColor)
	output += fmt.Sprintf("Magnification End: %f\n", cs.MagnificationEnd)
	output += fmt.Sprintf("Magnification Start: %f\n", cs.MagnificationStart)
	output += fmt.Sprintf("Magnification Step: %f\n", cs.MagnificationStep)
	output += fmt.Sprintf("Max Iterations: %d\n", cs.MaxIterations)
	output += fmt.Sprintf("Palette: %v\n", cs.Palette)
	output += fmt.Sprintf("Smooth Coloring: %t\n", cs.SmoothColoring)
	output += fmt.Sprintf("Super Sampling: %d\n", cs.SuperSampling)
	output += fmt.Sprintf("Width: %d\n", cs.Width)
	return output
}

func (cs *CoordinatorSettings) Verify() error {
	if cs.Boundary <= 0 {
		cs.Boundary = 100
	}
	if cs.CenterX < -4 || cs.CenterX > 4 {
		cs.CenterX = 0
	}
	if cs.CenterY < -4 || cs.CenterX > 4 {
		cs.CenterY = 0
	}
	// cs.EnableWebInterface defaults to false already
	if cs.EscapeColor == (color.RGBA{}) {
		cs.EscapeColor = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	if len(cs.GeneratePaletteSettings) > 0 {
		cs.Palette = cs.GeneratePalette(cs.GeneratePaletteSettings)
	}
	if cs.Height <= 0 {
		cs.Height = 1080
	}
	if cs.MagnificationEnd <= 0 {
		cs.MagnificationEnd = 1.5
	}
	if cs.MagnificationStart <= 0 {
		cs.MagnificationStart = 0.5
	}
	if cs.MagnificationStep <= 0 {
		cs.MagnificationStep = 1
	}
	if cs.MaxIterations <= 0 {
		cs.MaxIterations = 1000
	}
	if len(cs.Palette) == 0 {
		cs.Palette = []color.RGBA{{R: 255, G: 255, B: 255, A: 255}}
	}
	// cs.SmoothColoring defaults to false already
	if cs.SuperSampling < 1 {
		cs.SuperSampling = 1
	}
	if cs.Width <= 0 {
		cs.Width = 1920
	}

	// Smooth coloring wont work with one color
	if len(cs.Palette) == 1 && cs.SmoothColoring == true {
		cs.SmoothColoring = false
	}
	return nil
}

/* Coordinator */
type Coordinator struct {
	ImageCount   int
	ImageTasks   []*ImageTask
	Logger       *log.Logger
	Mutex        sync.Mutex
	ShorterSide  int
	Settings     *CoordinatorSettings
	TaskCount    int
	TasksDone    chan LineTask
	TaskSettings *TaskSettings
	TasksTodo    chan LineTask
	Wait         *sync.WaitGroup
	Workers      map[string]*rpc.Client
}

type ImageTask struct {
	Completed  bool
	Image      *image.RGBA
	PixelsLeft int
}

func newCoordinator(settings CoordinatorSettings, ipAddress string, port int) Coordinator {
	rect := image.Rectangle{
		Min: image.Point{X: 0, Y: 0},
		Max: image.Point{X: settings.Width, Y: settings.Height},
	}
	shorterSide := settings.Height
	if settings.Width < settings.Height {
		shorterSide = settings.Width
	}
	imageCount := int(math.Ceil((settings.MagnificationEnd - settings.MagnificationStart) / settings.MagnificationStep))

	coordinator := Coordinator{
		ImageCount: imageCount,
		ImageTasks: make([]*ImageTask, 0),
		Logger:     log.New(os.Stdout, fmt.Sprintf("Coordinator[%s:%d] ", ipAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		Settings: &CoordinatorSettings{
			Boundary:           settings.Boundary,
			CenterX:            settings.CenterX,
			CenterY:            settings.CenterY,
			EscapeColor:        settings.EscapeColor,
			Height:             settings.Height,
			MagnificationEnd:   settings.MagnificationEnd,
			MagnificationStart: settings.MagnificationStart,
			MagnificationStep:  settings.MagnificationStep,
			MaxIterations:      settings.MaxIterations,
			Palette:            settings.Palette,
			SmoothColoring:     settings.SmoothColoring,
			SuperSampling:      settings.SuperSampling,
			Width:              settings.Width,
		},
		ShorterSide: shorterSide,
		TaskCount:   settings.Height * imageCount,
		TasksDone:   make(chan LineTask, 1000),
		TaskSettings: &TaskSettings{
			Boundary:       settings.Boundary,
			CenterX:        settings.CenterX,
			CenterY:        settings.CenterY,
			EscapeColor:    settings.EscapeColor,
			Height:         settings.Height,
			MaxIterations:  settings.MaxIterations,
			Palette:        settings.Palette,
			SmoothColoring: settings.SmoothColoring,
			ShorterSide:    shorterSide,
			SuperSampling:  settings.SuperSampling,
			Width:          settings.Width,
		},
		TasksTodo: make(chan LineTask, 1000),
		Wait:      &sync.WaitGroup{},
		Workers:   make(map[string]*rpc.Client, 0),
	}

	pixelCount := coordinator.Settings.Height * coordinator.Settings.Width
	for c := 0; c < coordinator.ImageCount; c++ {
		imageTask := &ImageTask{
			Completed:  false,
			Image:      image.NewRGBA(rect),
			PixelsLeft: pixelCount,
		}
		coordinator.ImageTasks = append(coordinator.ImageTasks, imageTask)
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

	// All work is done
	if err.Error() == "all tasks handed out" {
		c.Logger.Print("All tasks handed out")
		return nil
	}

	c.Workers[workerAddress].Close()
	c.Logger.Printf("ERROR - Failed to call worker at address: %s, method: %s, error: %v", workerAddress, method, err)
	return err
}

func (c *Coordinator) GenerateTasks() {
	c.Logger.Print("Generating tasks")

	// for each picture to be generated
	imageNumber := 0
	for magnification := c.Settings.MagnificationStart; magnification < c.Settings.MagnificationEnd; magnification += c.Settings.MagnificationStep {

		// for each pixel in this particular image
		for row := 0; row < c.Settings.Height; row++ {
			task := LineTask{
				currentWidth:  0,
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

		imageNumber++
	}
	close(c.TasksTodo)

	c.Logger.Printf("Done generating %d tasks", c.TaskCount)
}

/* RPC */
func (c *Coordinator) RequestTask(request Nothing, reply *LineTask) error {
	c.Mutex.Lock()
	task, more := <-c.TasksTodo
	c.Mutex.Unlock()
	if !more {
		reply = nil
		c.Logger.Print("All tasks handed out")
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
	reply.CenterX = c.Settings.CenterX
	reply.CenterY = c.Settings.CenterY
	reply.EscapeColor = c.Settings.EscapeColor
	reply.Height = c.Settings.Height
	reply.MaxIterations = c.Settings.MaxIterations
	reply.Palette = c.Settings.Palette
	reply.ShorterSide = c.ShorterSide
	reply.SmoothColoring = c.Settings.SmoothColoring
	reply.SuperSampling = c.Settings.SuperSampling
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

/* Web Interface */
func (c *Coordinator) StartWebInterface() error {
	// parse all template files
	var allFiles []string
	files, _ := ioutil.ReadDir("./static/templates")

	for _, file := range files {
		filename := file.Name()
		if strings.HasSuffix(filename, ".html") {
			allFiles = append(allFiles, "./static/templates/"+filename)
		}
	}

	// todo: handle case where allFiles is empty
	templates, _ = template.New(filepath.Base(allFiles[0])).ParseFiles(allFiles...)

	// set up a file server for static files
	fileServer := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fileServer))

	http.HandleFunc("/", c.indexHandler)
	http.HandleFunc("/settings", c.settingsHandler)
	http.HandleFunc("/defaultSettings", c.defaultSettingsHandler)
	go http.ListenAndServe("localhost:8080", nil)
	c.Logger.Printf("Browser interface now running at localhost:8080")
	return nil
}

func (c *Coordinator) indexHandler(w http.ResponseWriter, r *http.Request) {
	type indexData struct {
		Settings           *TaskSettings
		MagnificationStart float64
		MagnificationEnd   float64
		MagnificationStep  float64
	}

	switch r.Method {
	case http.MethodGet:
		_ = templates.Execute(w, indexData{c.TaskSettings, c.Settings.MagnificationStart, c.Settings.MagnificationEnd, c.Settings.MagnificationStep})
		break
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) settingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, _ := json.Marshal(c.Settings)

		w.Header().Set("Content-Type", "application/json")
		w.Write(settings)
		break
	case http.MethodPost:
		// todo: figure out why this is not updating...
		_ = json.NewDecoder(r.Body).Decode(&c.Settings)
		break
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) defaultSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := make(map[string]string, 0)
		flag.VisitAll(func(flag *flag.Flag) {
			switch flag.Name {
			// Filter out values that dont need to be passed on
			case "coordinatorAddress":
			case "isCoordinator":
			case "isWorker":
			case "superSampling":
			case "workerCount":
				return
			default:
				settings[flag.Name] = flag.DefValue
			}
		})
		defaultSettings, _ := json.Marshal(settings)

		w.Header().Set("Content-Type", "application/json")
		w.Write(defaultSettings)
		break
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
