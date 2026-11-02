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
	"sync/atomic"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
)

// Coordinator hands out tasks to workers and assembles their results into images.
//
// Concurrency: clients and tasksHandedOut are guarded by mutex. images is only touched by the ingestTasks goroutine.
// The counters below are atomic because each has a single writer goroutine but is also read by the heartbeat ticker.
// Everything else, including completed and makeMovie, is set before start and only read afterwards.
type Coordinator struct {
	allTasksDone   chan struct{} // closed once every task has been ingested; tells waiting workers to stop
	clients        map[string]*multirpc.TcpClient
	completed      map[uint]bool // frames saved before this run was resumed
	digitCount     uint          // Used to format name of images for ffmpeg
	images         map[int]imageTask
	imageCount     uint
	logger         bslogger.Logger
	makeMovie      bool // GenerateMovie is set and ffmpeg was found at startup
	mutex          sync.Mutex
	name           string
	pixelCount     uint
	rectangle      gimage.Rectangle
	regions        []gimage.Rectangle // how each frame is split into tasks; set by planTasks
	settings       settings
	taskCount      uint
	tasksHandedOut map[string]map[uint]task.Task // keep track of all tasks workers have
	tasksDone      chan task.Task
	tasksTodo      chan task.Task
	workerWait     *sync.WaitGroup

	imageCompletedCount atomic.Uint64 // written by ingestTasks
	imagesInProgress    atomic.Int64  // written by ingestTasks; mirrors len(images) for the heartbeat
	taskGeneratedCount  atomic.Uint64 // written by generateTasks
	taskIngestedCount   atomic.Uint64 // written by ingestTasks

	Server multirpc.TcpServer
}

// NewCoordinator starts a new run with the settings in settingsFile
func NewCoordinator(settingsFile string) *Coordinator {
	coordinator := newCoordinator(NewSettings(settingsFile))
	misc.CheckError(coordinator.planTasks(), coordinator.logger, misc.Fatal)

	// Create directory to store files for this run
	misc.CheckError(os.MkdirAll(coordinator.runFolder(), os.ModePerm), coordinator.logger, misc.Fatal)

	// Copy the settings to the directory so the run can be duplicated or resumed in the future
	marshaledSettings, err := json.MarshalIndent(coordinator.settings, "", "    ")
	misc.CheckError(err, coordinator.logger, misc.Fatal)
	bytesWritten, err := misc.WriteFile(filepath.Join(coordinator.runFolder(), settingsBackupName), marshaledSettings)
	if err != nil || bytesWritten == 0 {
		coordinator.logger.Fatalf("Unable to make a backup copy of settingsFile %s: %s", settingsFile, err)
	}

	coordinator.useLogFile(os.O_TRUNC)
	coordinator.start()
	return coordinator
}

// ResumeCoordinator continues a run that stopped before all of its frames were saved. It uses the settings saved in
// runFolder and only renders the frames that are missing. If none are missing it goes straight to making the movie.
func ResumeCoordinator(runFolder string) *Coordinator {
	settings, err := loadRunSettings(runFolder)
	misc.CheckError(err, bslogger.NewLogger("Coordinator", bslogger.Normal, nil), misc.Fatal)
	coordinator := newCoordinator(settings)
	misc.CheckError(coordinator.planTasks(), coordinator.logger, misc.Fatal)

	coordinator.useLogFile(os.O_APPEND)
	coordinator.logger.Infof("Resuming run %s", coordinator.runFolder())
	coordinator.resumeFrom(coordinator.completedFrames())
	coordinator.logger.Infof("%d of %d frames were already saved, %d left to render",
		len(coordinator.completed), coordinator.imageCount, coordinator.imageCount-uint(len(coordinator.completed)))

	coordinator.start()
	return coordinator
}

// newCoordinator builds a Coordinator for verified settings without starting anything
func newCoordinator(settings settings) *Coordinator {
	return &Coordinator{
		allTasksDone: make(chan struct{}),
		clients:      make(map[string]*multirpc.TcpClient),
		images:       make(map[int]imageTask),
		logger:       bslogger.NewLogger("Coordinator", bslogger.Normal, nil),
		pixelCount:   settings.MandelbrotSettings.Height * settings.MandelbrotSettings.Width,
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
}

// useLogFile records the log in the run folder as well as on the console. flag is os.O_TRUNC to start a new log or
// os.O_APPEND to add to the log of a resumed run.
func (c *Coordinator) useLogFile(flag int) {
	logFile, err := os.OpenFile(filepath.Join(c.runFolder(), "coordinator.log"), os.O_CREATE|os.O_WRONLY|flag, 0644)
	misc.CheckError(err, c.logger, misc.Warning)
	c.logger = bslogger.NewLogger("Coordinator", bslogger.Normal, logFile)
}

// start begins serving workers and generating and ingesting tasks
func (c *Coordinator) start() {
	if c.settings.GenerateMovie {
		c.makeMovie = ffmpegAvailable()
		if !c.makeMovie {
			c.logger.Warning("ffmpeg was not found, so no movie will be made. Install it and resume this run to make the movie.")
		}
	}

	// Start up the rpc tcp server to allow workers to communicate with the coordinator. This must come after all other
	// setup: once Run returns, worker calls are handled on their own goroutines and may read any coordinator field.
	c.Server = multirpc.NewTcpServer(c, c.settings.ServerAddress, "CoordinatorServer")
	misc.CheckError(c.Server.Run(), c.logger, misc.Fatal)

	go c.tickers()
	go c.generateTasks()
	go c.ingestTasks()
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
			c.logger.Info(c.status())
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
			// Frames saved before a resume are skipped but still numbered, so later frames keep their file names
			if !c.completed[imageNumber] {
				currentX, currentY, magnification := transition.frame(currentFrame)
				frame := task.Frame{CenterX: currentX, CenterY: currentY, Magnification: magnification}

				for _, region := range c.regions {
					c.tasksTodo <- c.newTask(imageNumber, frame, region)
				}
			}

			imageNumber++
		}
	}

	elapsedTime = time.Since(startTime)
	// tasksTodo is intentionally left open: tasks from workers that leave are put back into it until all are done

	c.logger.Infof("Done generating %d tasks in %s", c.taskGeneratedCount.Load(), elapsedTime.Round(time.Second).String())
}

func (c *Coordinator) ingestTasks() {
	c.logger.Info("Ingesting tasks")

	var elapsedTime time.Duration
	var startTime = time.Now()

	for c.taskIngestedCount.Load() < uint64(c.taskCount) {
		c.ingestTask(<-c.tasksDone)
	}

	elapsedTime = time.Since(startTime)
	// Release workers waiting in GetTask. tasksDone stays open so a late ReturnTask can never panic. Closing under the
	// mutex means RegisterWorker either finished its workerWait.Add before this point or will see the close and refuse.
	c.mutex.Lock()
	close(c.allTasksDone)
	workersLeft := len(c.clients)
	c.mutex.Unlock()
	c.logger.Infof("Done ingesting %d tasks in %s", c.taskIngestedCount.Load(), elapsedTime.Round(time.Second).String())

	c.logger.Infof("Waiting for %d workers to disconnect", workersLeft)
	c.workerWait.Wait()

	if c.makeMovie {
		c.generateMovie()
	}

	c.logger.Info("Shutting Down")
	misc.CheckError(c.Server.Stop(), c.logger, misc.Warning)
}

// ingestTask records a finished task's pixels on its image and saves the image once every pixel has arrived. It must
// only be called from the ingestTasks goroutine, which owns the images map, so no locking is needed. ReturnTask has
// already checked that Pixels holds exactly one color per pixel of Bounds.
func (c *Coordinator) ingestTask(taskReceived task.Task) {
	c.taskIngestedCount.Add(1)
	imageNumber := int(taskReceived.ImageNumber)

	image, ok := c.images[imageNumber]
	if !ok {
		// Need to create an image to save the incoming pixels
		image = imageTask{
			Image:      gimage.NewRGBA(c.rectangle),
			PixelsLeft: c.pixelCount,
		}
	}

	// Copy the pixels onto the image one row at a time; Pixels uses the same packed RGBA layout as image.Pix and
	// decrement the amount of pixels left to be recorded
	bounds := taskReceived.Bounds
	rowBytes := bounds.Dx() * task.BytesPerPixel
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		destination := image.Image.PixOffset(bounds.Min.X, y)
		source := (y - bounds.Min.Y) * rowBytes
		copy(image.Image.Pix[destination:destination+rowBytes], taskReceived.Pixels[source:source+rowBytes])
	}
	image.PixelsLeft -= uint(taskReceived.PixelCount())

	if image.PixelsLeft > 0 {
		c.images[imageNumber] = image
		c.imagesInProgress.Store(int64(len(c.images)))
		return
	}

	// All pixels have been recorded so save the image
	path := c.framePath(taskReceived.ImageNumber)
	if err := saveFrame(path, image.Image); err != nil {
		c.logger.Fatalf("ERROR - Unable to save image: %s", err)
	}
	c.logger.Infof("Saved image to %s", path)

	// Remove the image to conserve memory
	delete(c.images, imageNumber)
	c.imagesInProgress.Store(int64(len(c.images)))
	c.imageCompletedCount.Add(1)
}

// status summarizes progress for the heartbeat log. It only reads atomic counters and fields that never change after
// startup, so it is safe to call from any goroutine.
func (c *Coordinator) status() string {
	completed := c.imageCompletedCount.Load()
	return fmt.Sprintf("Tasks [Generated: %d] [Ingested: %d] | Images [Completed: %d] [WIP: %d] [Todo: %d]",
		c.taskGeneratedCount.Load(), c.taskIngestedCount.Load(), completed, c.imagesInProgress.Load(), uint64(c.imageCount)-completed)
}

// newTask creates a task with the next ID. Only generateTasks creates tasks, but the count is also read by status.
func (c *Coordinator) newTask(imageNumber uint, frame task.Frame, bounds gimage.Rectangle) task.Task {
	id := c.taskGeneratedCount.Add(1) - 1
	return task.NewTask(uint(id), imageNumber, frame, bounds)
}

// planTasks works out how many frames each transition has, how each frame is split into task regions, and the total
// number of tasks. The coordinator shuts down once that many tasks are ingested, so the count comes from the same
// regions generateTasks uses and cannot drift from what is actually generated.
func (c *Coordinator) planTasks() error {
	c.imageCount = 0
	for i := range c.settings.TransitionSettings {
		c.settings.TransitionSettings[i].FrameCount = c.settings.TransitionSettings[i].frameCount()
		c.imageCount += c.settings.TransitionSettings[i].FrameCount
	}

	// ffmpeg needs the images named in a certain way
	c.digitCount = uint(math.Log10(float64(c.imageCount)) + 1)

	c.regions = task.Regions(c.settings.TaskGeneration, c.settings.MandelbrotSettings.Width, c.settings.MandelbrotSettings.Height)
	if len(c.regions) == 0 {
		return fmt.Errorf("unknown generation type: %d", c.settings.TaskGeneration)
	}
	c.taskCount = uint(len(c.regions)) * c.imageCount
	return nil
}

// resumeFrom leaves out the frames an earlier attempt at this run already saved. It must be called after planTasks.
// Frame numbers stay the same as in the full run, so the saved and new frames form one sequence for ffmpeg.
func (c *Coordinator) resumeFrom(completed map[uint]bool) {
	c.completed = completed
	c.taskCount = uint(len(c.regions)) * (c.imageCount - uint(len(completed)))
	c.imageCompletedCount.Store(uint64(len(completed)))
}

// completedFrames finds the frames an earlier attempt at this run already saved. A frame only counts if it decodes
// completely at this run's size, since runs from before frames were saved atomically can leave a truncated file.
func (c *Coordinator) completedFrames() map[uint]bool {
	completed := make(map[uint]bool)
	for imageNumber := uint(1); imageNumber <= c.imageCount; imageNumber++ {
		path := c.framePath(imageNumber)
		f, err := os.Open(path)
		if err != nil {
			continue // not saved yet
		}
		frame, err := jpeg.Decode(f)
		misc.CheckError(f.Close(), c.logger, misc.Warning)
		switch {
		case err != nil:
			c.logger.Warningf("%s is incomplete and will be rendered again: %s", path, err)
		case frame.Bounds() != c.rectangle:
			c.logger.Warningf("%s is %v instead of %v and will be rendered again", path, frame.Bounds().Size(), c.rectangle.Size())
		default:
			completed[imageNumber] = true
		}
	}
	return completed
}

// saveFrame writes the frame under a temporary name and then renames it, so a frame file that exists is always
// complete even if the coordinator is stopped while saving it
func saveFrame(path string, frame gimage.Image) error {
	temporary := path + ".tmp"
	f, err := os.Create(temporary)
	if err != nil {
		return err
	}
	err = jpeg.Encode(f, frame, nil)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}

func (c *Coordinator) runFolder() string {
	return filepath.Join(c.settings.SavePath, c.settings.RunName)
}

// framePath is where a frame is saved. ffmpeg needs the frame numbers zero padded to the same width.
func (c *Coordinator) framePath(imageNumber uint) string {
	return filepath.Join(c.runFolder(), fmt.Sprintf("%0[1]*[2]d.jpg", c.digitCount, imageNumber))
}

func (c *Coordinator) generateMovie() {
	c.logger.Info("Making movie")
	args := []string{"-r", "60", "-i", filepath.Join(c.runFolder(), fmt.Sprintf("%%0%dd.jpg", c.digitCount)), "-c:v", "libx264", "-pix_fmt", "yuvj420p", filepath.Join(c.runFolder(), "movie.mp4")}
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	misc.CheckError(cmd.Run(), c.logger, misc.Error)
	c.logger.Info("Done making movie")
}

func (c *Coordinator) RegisterWorker(registration task.Registration, reply *misc.Nothing) error {
	workerServerAddress := registration.Address
	if registration.ProtocolVersion != task.ProtocolVersion {
		return fmt.Errorf("worker %s uses protocol version %d but the coordinator uses version %d; run the same build on both",
			workerServerAddress, registration.ProtocolVersion, task.ProtocolVersion)
	}

	// Create a client to communicate with this worker
	client := multirpc.NewTcpClient(workerServerAddress, workerServerAddress)
	c.mutex.Lock()
	if _, registered := c.clients[workerServerAddress]; registered {
		c.mutex.Unlock()
		return fmt.Errorf("worker %s is already registered", workerServerAddress)
	}
	select {
	case <-c.allTasksDone:
		// ingestTasks may already be waiting for workers to leave, so a new worker must not be added
		c.mutex.Unlock()
		return errors.New(task.NoMoreTasksMessage)
	default:
	}
	c.clients[workerServerAddress] = &client
	// Track all tasks this worker checks out
	c.tasksHandedOut[workerServerAddress] = make(map[uint]task.Task)
	// Add to the wait group while holding the mutex: the matching Done in DeRegisterWorker takes the same mutex so it
	// can never run first, and allTasksDone is closed under the mutex so this always happens before ingestTasks waits
	c.workerWait.Add(1)
	c.mutex.Unlock()
	misc.CheckError(client.Connect(), c.logger, misc.Warning)

	c.logger.Infof("Worker joined: %s", workerServerAddress)

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
//
// Only the pixel data is taken from the worker; the image number and bounds come from the coordinator's own record of
// the task. A result with the wrong amount of pixel data is rejected with an error and the task stays assigned, so the
// worker stops and the task is handed out again when the worker deregisters.
func (c *Coordinator) ReturnTask(done task.Task, nothing *misc.Nothing) error {
	c.mutex.Lock()
	assigned, handedOut := c.tasksHandedOut[done.WorkerAddress][done.ID]
	if handedOut {
		assigned.Pixels = done.Pixels
		if !assigned.HasValidResult() {
			c.mutex.Unlock()
			return fmt.Errorf("task %d returned %d bytes of pixel data but needs %d", done.ID, len(done.Pixels), assigned.ResultSize())
		}
		delete(c.tasksHandedOut[done.WorkerAddress], done.ID)
	}
	c.mutex.Unlock()

	if !handedOut {
		c.logger.Warningf("Ignoring task %d from worker %s since it is no longer assigned to that worker", done.ID, done.WorkerAddress)
		return nil
	}
	c.tasksDone <- assigned
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
