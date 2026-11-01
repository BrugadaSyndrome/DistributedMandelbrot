package coordinator

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	gimage "image"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
)

type Coordinator struct {
	allTasksDone        chan struct{} // closed once every task has been ingested; tells waiting workers to stop
	clients             map[string]*multirpc.TcpClient
	digitCount          uint // Used to format name of images for ffmpeg
	images              map[int]imageTask
	imageCompletedCount uint
	imageCount          uint
	logger              bslogger.Logger
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

	Server multirpc.TcpServer
}

func NewCoordinator(settingsFile string) *Coordinator {
	settings := NewSettings(settingsFile)

	coordinator := &Coordinator{
		allTasksDone: make(chan struct{}),
		clients:      make(map[string]*multirpc.TcpClient),
		images:     make(map[int]imageTask),
		logger:     bslogger.NewLogger("Coordinator", bslogger.Normal, nil),
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

	// Determine the number of images that will be generated for each transition (see transitionSettings.frameCount)
	for i := range settings.TransitionSettings {
		settings.TransitionSettings[i].FrameCount = settings.TransitionSettings[i].frameCount()
		coordinator.imageCount += settings.TransitionSettings[i].FrameCount
	}

	// ffmpeg needs the images named in a certain way
	coordinator.digitCount = (uint)(math.Log10((float64)(coordinator.imageCount)) + 1)

	// Determine the number of tasks that will be generated so the coordinator knows when to shut down
	switch settings.TaskGeneration {
	case task.Row:
		coordinator.taskCount = settings.MandelbrotSettings.Height * coordinator.imageCount
	case task.Column:
		coordinator.taskCount = settings.MandelbrotSettings.Width * coordinator.imageCount
	case task.Image:
		coordinator.taskCount = coordinator.imageCount
	case task.Grid:
		coordinator.taskCount = task.GridDivisions * task.GridDivisions * coordinator.imageCount
	default:
		coordinator.logger.Fatalf("Unknown generation type: %d", coordinator.settings.TaskGeneration)
		break
	}

	// Start up the rpc tcp server to allow workers to communicate with the coordinator
	coordinator.Server = multirpc.NewTcpServer(coordinator, settings.ServerAddress, "CoordinatorServer")
	misc.CheckError(coordinator.Server.Run(), coordinator.logger, misc.Fatal)

	// Create directory to store files for this run
	if _, err := os.Stat(filepath.Join(settings.SavePath, settings.RunName)); os.IsNotExist(err) {
		err = os.Mkdir(filepath.Join(settings.SavePath, settings.RunName), os.ModePerm)
		if err != nil {
			coordinator.logger.Fatalf("Unable to create folder: %s", err)
		}

	}

	// Copy the settings to the directory so the run can be duplicated in the future
	marshaledSettings, err := json.Marshal(settings)
	bytesWritten, err := misc.WriteFile(filepath.Join(settings.SavePath, settings.RunName, settingsFile), marshaledSettings)
	if err != nil || bytesWritten == 0 {
		coordinator.logger.Fatalf("Unable to make a backup copy of settingsFile: %s", settingsFile)
	}

	// Create a log file to record the run
	logFile, err := os.Create(filepath.Join(settings.SavePath, settings.RunName, "coordinator.log"))
	misc.CheckError(err, coordinator.logger, misc.Warning)
	coordinator.logger = bslogger.NewLogger("Coordinator", bslogger.Normal, logFile)

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
			// Copy the clients so workers can register and deregister while the roll call is in progress
			c.mutex.Lock()
			clients := make([]*multirpc.TcpClient, 0, len(c.clients))
			for _, v := range c.clients {
				clients = append(clients, v)
			}
			c.mutex.Unlock()
			for _, v := range clients {
				var reply bool
				err := v.Call("Worker.RollCall", junk, &reply)
				if err != nil {
					// Cannot communicate with the worker
					c.logger.Warningf("Worker %s missed roll call: %s", v.Name(), err)

					// Remove worker from pool, which also disconnects from it
					var nothing misc.Nothing
					misc.CheckError(c.DeRegisterWorker(v.Name(), &nothing), c.logger, misc.Warning)
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
		// generate each image for this transition while zooming exponentially
		transition := c.settings.TransitionSettings[transitionStep]

		for currentFrame := range transition.FrameCount {
			currentX, currentY, magnification := transition.frame(currentFrame)

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
			case task.Grid:
				var gridRow, gridColumn uint
				for gridRow = range task.GridDivisions {
					for gridColumn = range task.GridDivisions {
						taskTodo := task.NewTask(c.taskGeneratedCount, imageNumber)
						taskTodo.AddTasksForImageByGrid(currentX, currentY, magnification, c.settings.MandelbrotSettings.Height, c.settings.MandelbrotSettings.Width, gridRow, gridColumn)
						c.tasksTodo <- taskTodo
						c.taskGeneratedCount++
					}
				}
			default:
				c.logger.Fatalf("Unknown generation type: %d", c.settings.TaskGeneration)
				break
			}

			imageNumber++
		}
	}

	elapsedTime = time.Since(startTime)
	// tasksTodo is intentionally left open: tasks from workers that leave are put back into it until all are done

	c.logger.Infof("Done generating %d tasks in %s", c.taskGeneratedCount, elapsedTime.Round(time.Second).String())
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
			c.mutex.Lock()
			c.images[int(taskReceived.ImageNumber)] = image
			c.mutex.Unlock()

			// All pixels have been recorded so save the image
			if image.PixelsLeft == 0 {
				path := filepath.Join(c.settings.SavePath, c.settings.RunName, fmt.Sprintf("%0[1]*[2]d.jpg", c.digitCount, taskReceived.ImageNumber))
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
				c.mutex.Lock()
				delete(c.images, int(taskReceived.ImageNumber))
				c.mutex.Unlock()
				c.imageCompletedCount++
			}
		}
	}

	elapsedTime = time.Since(startTime)
	// Release workers waiting in GetTask. tasksDone stays open so a late ReturnTask can never panic.
	close(c.allTasksDone)
	c.logger.Infof("Done ingesting %d tasks in %s", c.taskIngestedCount, elapsedTime.Round(time.Second).String())

	c.logger.Infof("Waiting for %d workers to disconnect", len(c.clients))
	c.workerWait.Wait()

	if c.settings.GenerateMovie {
		c.generateMovie()
	}

	c.logger.Info("Shutting Down")
	misc.CheckError(c.Server.Stop(), c.logger, misc.Warning)
}

func (c *Coordinator) generateMovie() {
	c.logger.Info("Making movie")
	args := []string{"-r", "60", "-i", filepath.Join(c.settings.SavePath, c.settings.RunName, fmt.Sprintf("%%%dd.jpg", c.digitCount)), "-c:v", "libx264", "-pix_fmt", "yuvj420p", filepath.Join(c.settings.SavePath, c.settings.RunName, "movie.mp4")}
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	misc.CheckError(cmd.Run(), c.logger, misc.Error)
	c.logger.Info("Done making movie")
}

func (c *Coordinator) RegisterWorker(workerServerAddress string, reply *misc.Nothing) error {
	// Create a client to communicate with this worker
	client := multirpc.NewTcpClient(workerServerAddress, workerServerAddress)
	c.mutex.Lock()
	c.clients[workerServerAddress] = &client
	// Track all tasks this worker checks out
	c.tasksHandedOut[workerServerAddress] = make(map[uint]task.Task)
	c.mutex.Unlock()
	misc.CheckError(client.Connect(), c.logger, misc.Warning)

	c.logger.Infof("Worker joined: %s", workerServerAddress)
	c.workerWait.Add(1)

	return nil
}

func (c *Coordinator) DeRegisterWorker(workerServerAddress string, reply *misc.Nothing) error {
	c.mutex.Lock()
	client, registered := c.clients[workerServerAddress]
	if !registered {
		// e.g. a worker removed after missing a roll call that later deregisters itself
		c.mutex.Unlock()
		return fmt.Errorf("worker %s is not registered", workerServerAddress)
	}
	unfinished := make([]task.Task, 0, len(c.tasksHandedOut[workerServerAddress]))
	for _, todo := range c.tasksHandedOut[workerServerAddress] {
		unfinished = append(unfinished, todo)
	}
	// Remove stored values associated with this worker
	delete(c.tasksHandedOut, workerServerAddress)
	delete(c.clients, workerServerAddress)
	c.mutex.Unlock()

	misc.CheckError(client.Disconnect(), c.logger, misc.Warning)
	// Put tasks this worker has not returned yet back into the tasksTodo pool
	c.requeue(unfinished)

	c.logger.Infof("Worker left: %s (%d unfinished tasks requeued)", workerServerAddress, len(unfinished))
	c.workerWait.Done()

	return nil
}

// requeue puts tasks back into tasksTodo in the background, since tasksTodo may be full and the caller should not
// block (or hold the mutex) until workers make room
func (c *Coordinator) requeue(tasks []task.Task) {
	if len(tasks) == 0 {
		return
	}
	go func() {
		for _, todo := range tasks {
			c.tasksTodo <- todo
		}
	}()
}

func (c *Coordinator) RollCall(nothing misc.Nothing, present *bool) error {
	*present = true
	return nil
}

// GetTask hands the worker the next task. When no task is queued it waits, because a worker that leaves may still
// have tasks that need to be handed out again, and only reports that there is no more work once every task is done.
func (c *Coordinator) GetTask(workerAddress string, assigned *task.Task) error {
	if !c.isRegistered(workerAddress) {
		return fmt.Errorf("worker %s is not registered", workerAddress)
	}

	select {
	case todo := <-c.tasksTodo:
		c.mutex.Lock()
		handedOut, registered := c.tasksHandedOut[workerAddress]
		if registered {
			todo.WorkerAddress = workerAddress
			handedOut[todo.ID] = todo
		}
		c.mutex.Unlock()
		if !registered {
			// The worker was removed while waiting for a task
			c.requeue([]task.Task{todo})
			return fmt.Errorf("worker %s is not registered", workerAddress)
		}
		*assigned = todo
		return nil
	case <-c.allTasksDone:
		c.logger.Infof("Telling worker %s that all tasks are done", workerAddress)
		return errors.New(task.NoMoreTasksMessage)
	}
}

// ReturnTask accepts a finished task only if it is still handed out to the worker returning it. A task taken back from
// a worker (e.g. one that missed a roll call) is handed out again, so a late copy from the original worker, or a
// repeated return, is ignored to keep its pixels from being counted twice.
func (c *Coordinator) ReturnTask(done task.Task, nothing *misc.Nothing) error {
	c.mutex.Lock()
	_, handedOut := c.tasksHandedOut[done.WorkerAddress][done.ID]
	delete(c.tasksHandedOut[done.WorkerAddress], done.ID)
	c.mutex.Unlock()

	if !handedOut {
		c.logger.Warningf("Ignoring task %d from worker %s since it is no longer assigned to that worker", done.ID, done.WorkerAddress)
		return nil
	}
	c.tasksDone <- done
	return nil
}

func (c *Coordinator) isRegistered(workerAddress string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	_, registered := c.tasksHandedOut[workerAddress]
	return registered
}

func (c *Coordinator) GetMandelbrotSettings(nothing misc.Nothing, settings *mandelbrot.Settings) error {
	*settings = c.settings.MandelbrotSettings
	return nil
}
