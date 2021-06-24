package coordinator

import (
	"encoding/json"
	"errors"
	"fmt"
	gimage "image"
	"image/jpeg"
	glog "log"
	"mandelbrot/log"
	"mandelbrot/mandelbrot"
	"mandelbrot/misc"
	"mandelbrot/rpc"
	"mandelbrot/task"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Coordinator struct {
	clients             map[string]*rpc.TcpClient
	images              map[int]imageTask
	imageCompletedCount uint
	imageCount          uint
	logger              log.Logger
	mutex               sync.Mutex
	name                string
	pixelCount          uint
	rectangle           gimage.Rectangle
	settings            settings
	taskCount           uint
	taskGeneratedCount  uint
	taskIngestedCount   uint
	tasksHandedOut      map[string]map[uint]task.Task // keep track of all tasks workers have
	tasksDone           chan task.Task
	tasksTodo           chan task.Task
	workerWait          *sync.WaitGroup

	Server rpc.TcpServer
}

// todo: handle generate movie option
func NewCoordinator(settingsFile string) Coordinator {
	settings := NewSettings(settingsFile)

	coordinator := Coordinator{
		clients:    make(map[string]*rpc.TcpClient),
		images:     make(map[int]imageTask),
		logger:     log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "Coordinator", log.Normal, nil),
		pixelCount: settings.MandelbrotSettings.Height * settings.MandelbrotSettings.Width,
		rectangle: gimage.Rectangle{
			Min: gimage.Point{
				X: 0,
				Y: 0,
			},
			Max: gimage.Point{
				X: int(settings.MandelbrotSettings.Width),
				Y: int(settings.MandelbrotSettings.Height),
			},
		},
		settings:       settings,
		tasksHandedOut: make(map[string]map[uint]task.Task),
		tasksDone:      make(chan task.Task, 1000),
		tasksTodo:      make(chan task.Task, 1000),
		workerWait:     &sync.WaitGroup{},
	}
	misc.CheckError(settings.Verify(), coordinator.logger, misc.Fatal)

	/*
	 * Use logarithms to determine the number of images that will be generated
	 *
	 * i.e.
	 * magnification_start + magnification_step^n = magnification_end
	 * log(magnification_start) + n = log_magnification_step(magnification_end)
	 * n = (log(magnification_end) / log(magnification_step)) - log(magnification_start)
	 */
	for i := 0; i < len(settings.TransitionSettings); i++ {
		var transitionCount uint = 1
		if settings.TransitionSettings[i].MagnificationStart < settings.TransitionSettings[i].MagnificationEnd {
			// zooming in
			transitionCount = uint(math.Ceil((math.Log(settings.TransitionSettings[i].MagnificationEnd) / math.Log(settings.TransitionSettings[i].MagnificationStep)) - math.Log(settings.TransitionSettings[i].MagnificationStart)))
		} else {
			// zooming out
			transitionCount = uint(math.Ceil((math.Log(settings.TransitionSettings[i].MagnificationStart) / math.Log(settings.TransitionSettings[i].MagnificationStep)) - math.Log(settings.TransitionSettings[i].MagnificationEnd)))
		}
		coordinator.imageCount += transitionCount
		settings.TransitionSettings[i].FrameCount = transitionCount
	}

	// Determine the number of tasks that will be generated so the coordinator knows when to shut down
	switch settings.TaskGeneration {
	case task.Row:
		coordinator.taskCount = settings.MandelbrotSettings.Height * coordinator.imageCount
	case task.Column:
		coordinator.taskCount = settings.MandelbrotSettings.Width * coordinator.imageCount
	case task.Image:
		coordinator.taskCount = coordinator.imageCount
	default:
		coordinator.logger.Fatalf("Unknown generation type: %d", coordinator.settings.TaskGeneration)
		break
	}

	// Start up the rpc tcp server to allow workers to communicate with the coordinator
	coordinator.Server = rpc.NewTcpServer(&coordinator, settings.ServerAddress, "CoordinatorServer")
	misc.CheckError(coordinator.Server.Run(), coordinator.Server.Logger, misc.Fatal)

	// Create directory to store files for this run
	if _, err := os.Stat(filepath.Join(settings.SavePath, settings.RunName)); os.IsNotExist(err) {
		err = os.Mkdir(filepath.Join(settings.SavePath, settings.RunName), os.ModePerm)
		if err != nil {
			coordinator.logger.Fatalf("Unable to create folder: %s", err)
		}

	}

	// Copy the settings to the directory so the run can be duplicated in the future
	bytes, err := json.Marshal(settings)
	bytesWritten, err := misc.WriteFile(filepath.Join(settings.SavePath, settings.RunName, settingsFile), bytes)
	if err != nil || bytesWritten == 0 {
		coordinator.logger.Fatalf("Unable to make a backup copy of settingsFile: %s", settingsFile)
	}

	// Create a log file to record the run
	logFile, err := os.Create(filepath.Join(settings.SavePath, settings.RunName, "coordinator.log"))
	misc.CheckError(err, coordinator.logger, misc.Warning)
	coordinator.logger = log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "Coordinator", log.Normal, logFile)

	go coordinator.tickers()
	go coordinator.generateTasks()
	go coordinator.ingestTasks()

	return coordinator
}

func (c *Coordinator) tickers() {
	rollCall := time.NewTicker(time.Minute)
	heartBeat := time.NewTicker(30 * time.Second)

	for {
		select {
		case _ = <-rollCall.C:
			c.logger.Debug("Roll call ticker")
			var junk misc.Nothing
			for _, v := range c.clients {
				var reply bool
				err := v.Call("Worker.RollCall", junk, &reply)
				if err != nil {
					// Cannot communicate with the worker
					c.logger.Warningf("Worker %s missed roll call: %s", v.Name, err)
					misc.CheckError(v.Disconnect(), c.logger, misc.Warning)

					// Remove worker from pool
					var nothing misc.Nothing
					misc.CheckError(c.DeRegisterWorker(v.Name, &nothing), c.logger, misc.Warning)
				}
			}

		case _ = <-heartBeat.C:
			c.logger.Debug("Heart beat ticker")
			c.logger.Infof("Tasks [Generated: %d] [Ingested: %d] | Images [Completed: %d] [WIP: %d] [Todo: %d]", c.taskGeneratedCount, c.taskIngestedCount, c.imageCompletedCount, len(c.images), c.imageCount-c.imageCompletedCount)
		}
	}
}

func (c *Coordinator) generateTasks() {
	c.logger.Info("Generating tasks")

	// Generate tasks for this image
	var imageNumber uint = 1
	var elapsedTime time.Duration
	var startTime = time.Now()

	for transitionStep := 0; transitionStep < len(c.settings.TransitionSettings); transitionStep++ {

		// generate each image for this transition while zooming in exponentially
		transition := c.settings.TransitionSettings[transitionStep]
		magnification := transition.MagnificationStart
		currentX := transition.StartX
		currentY := transition.StartY

		var currentFrame uint
		for currentFrame = 1; currentFrame <= transition.FrameCount; currentFrame++ {

			// Lerp through the coordinates in the transition
			t := float64(currentFrame) / float64(transition.FrameCount)

			// zooming out
			if transition.MagnificationStart > transition.MagnificationEnd {
				currentX = misc.LerpFloat64(transition.StartX, transition.EndX, misc.EaseInExpo(t))
				currentY = misc.LerpFloat64(transition.StartY, transition.EndY, misc.EaseInExpo(t))
				magnification /= transition.MagnificationStep
			}

			switch c.settings.TaskGeneration {
			case task.Row:
				var row uint
				for row = 0; row < c.settings.MandelbrotSettings.Height; row++ {
					taskTodo := task.NewTask(c.taskGeneratedCount, imageNumber)
					taskTodo.AddTasksForRow(currentX, currentY, magnification, row, c.settings.MandelbrotSettings.Width)
					c.tasksTodo <- taskTodo
					c.taskGeneratedCount++
				}
			case task.Column:
				var column uint
				for column = 0; column < c.settings.MandelbrotSettings.Width; column++ {
					taskTodo := task.NewTask(c.taskGeneratedCount, imageNumber)
					taskTodo.AddTasksForColumn(currentX, currentY, magnification, c.settings.MandelbrotSettings.Height, column)
					c.tasksTodo <- taskTodo
					c.taskGeneratedCount++
				}
			case task.Image:
				taskTodo := task.NewTask(c.taskGeneratedCount, imageNumber)
				taskTodo.AddTasksForImage(currentX, currentY, magnification, c.settings.MandelbrotSettings.Height, c.settings.MandelbrotSettings.Width)
				c.tasksTodo <- taskTodo
				c.taskGeneratedCount++
			default:
				c.logger.Fatalf("Unknown generation type: %d", c.settings.TaskGeneration)
				break
			}

			// zooming in
			if transition.MagnificationStart < transition.MagnificationEnd {
				currentX = misc.LerpFloat64(transition.StartX, transition.EndX, misc.EaseOutExpo(t))
				currentY = misc.LerpFloat64(transition.StartY, transition.EndY, misc.EaseOutExpo(t))
				magnification *= transition.MagnificationStep
			}

			imageNumber++
		}
	}

	elapsedTime = time.Since(startTime)
	close(c.tasksTodo)

	c.logger.Debugf("Done generating %d tasks in %s", c.taskGeneratedCount, elapsedTime)
}

func (c *Coordinator) ingestTasks() {
	c.logger.Info("Ingesting tasks")

	var elapsedTime time.Duration
	var startTime = time.Now()

	for {
		if c.taskIngestedCount == c.taskCount {
			// There are no more tasks to ingest
			break
		}

		// Get the next task to work on
		taskReceived, _ := <-c.tasksDone
		c.taskIngestedCount++

		for r := 0; r < len(taskReceived.Results); r++ {
			image, ok := c.images[int(taskReceived.ImageNumber)]
			if !ok {
				// Need to create an image save the incoming pixels
				image = imageTask{
					Image:      gimage.NewRGBA(c.rectangle),
					PixelsLeft: c.pixelCount,
				}
			}

			// Record the pixel on the image and decrement the amount of pixels left to be recorded
			result := taskReceived.Results[r]
			image.Image.SetRGBA(int(result.Column), int(result.Row), result.Color)
			image.PixelsLeft--
			c.images[int(taskReceived.ImageNumber)] = image
			c.mutex.Lock()
			delete(c.tasksHandedOut[taskReceived.WorkerAddress], taskReceived.ID)
			c.mutex.Unlock()

			// All pixels have been recorded so save the image
			if image.PixelsLeft == 0 {
				path := filepath.Join(c.settings.SavePath, c.settings.RunName, fmt.Sprintf("%d.jpg", taskReceived.ImageNumber))
				f, err := os.Create(path)
				if err != nil {
					c.logger.Fatalf("ERROR - Unable to create image: %s", err)
				}
				err = jpeg.Encode(f, image.Image, nil)
				if err != nil {
					c.logger.Fatalf("ERROR - Unable to save image: %s", err)
				}
				c.logger.Infof("Saved image to %s", path)

				// Remove the image to conserve memory
				delete(c.images, int(taskReceived.ImageNumber))
				c.imageCompletedCount++
			}
		}
	}

	elapsedTime = time.Since(startTime)
	close(c.tasksDone)
	c.logger.Debugf("Done ingesting %d tasks in %s", c.taskIngestedCount, elapsedTime)

	c.logger.Infof("Waiting for %d workers to disconnect", len(c.clients))
	c.workerWait.Wait()
	misc.CheckError(c.Server.Stop(), c.logger, misc.Warning)
}

func (c *Coordinator) RegisterWorker(workerServerAddress string, reply *misc.Nothing) error {
	// Create a client to communicate with this worker
	client := rpc.NewTcpClient(workerServerAddress, workerServerAddress)
	c.clients[workerServerAddress] = &client
	misc.CheckError(client.Connect(), c.logger, misc.Warning)

	// Track all tasks this worker checks out
	c.tasksHandedOut[workerServerAddress] = make(map[uint]task.Task)

	c.logger.Infof("Worker joined: %s", workerServerAddress)
	c.workerWait.Add(1)

	return nil
}

func (c *Coordinator) DeRegisterWorker(workerServerAddress string, reply *misc.Nothing) error {
	// Put tasks  this worker has not returned yet back into the tasksTodo pool
	go func(tasks map[uint]task.Task) {
		for _, v := range tasks {
			c.tasksTodo <- v
		}
	}(c.tasksHandedOut[workerServerAddress])

	// Disconnect from worker
	misc.CheckError(c.clients[workerServerAddress].Disconnect(), c.logger, misc.Warning)

	// Remove stored values associated with this worker
	c.mutex.Lock()
	delete(c.tasksHandedOut, workerServerAddress)
	delete(c.clients, workerServerAddress)
	c.mutex.Unlock()

	c.logger.Infof("Worker left: %s", workerServerAddress)
	c.workerWait.Done()

	return nil
}

func (c *Coordinator) RollCall(nothing misc.Nothing, present *bool) error {
	*present = true
	return nil
}

func (c *Coordinator) GetTask(workerAddress string, task *task.Task) error {
	todo, more := <-c.tasksTodo
	if !more {
		task = nil
		c.logger.Info("Telling worker that all tasks are handed out")
		return errors.New("all tasks handed out")
	}
	c.mutex.Lock()
	todo.WorkerAddress = workerAddress
	c.tasksHandedOut[workerAddress][todo.ID] = todo
	c.mutex.Unlock()
	*task = todo
	return nil
}

func (c *Coordinator) ReturnTask(done task.Task, nothing *misc.Nothing) error {
	c.tasksDone <- done
	return nil
}

func (c *Coordinator) GetMandelbrotSettings(nothing misc.Nothing, settings *mandelbrot.Settings) error {
	*settings = c.settings.MandelbrotSettings
	return nil
}
