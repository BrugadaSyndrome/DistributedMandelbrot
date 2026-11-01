package coordinator

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"fmt"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
)

// newTestCoordinator builds a Coordinator with only the state used by the worker facing RPC methods. No servers or
// background goroutines are started.
func newTestCoordinator() *Coordinator {
	return &Coordinator{
		allTasksDone:   make(chan struct{}),
		clients:        make(map[string]*multirpc.TcpClient),
		logger:         bslogger.NewLogger("CoordinatorTest", bslogger.Normal, nil),
		tasksHandedOut: make(map[string]map[uint]task.Task),
		tasksDone:      make(chan task.Task, 10),
		tasksTodo:      make(chan task.Task, 10),
		workerWait:     &sync.WaitGroup{},
	}
}

// addTestWorker registers a worker the same way RegisterWorker does but without connecting to it
func addTestWorker(c *Coordinator, address string) {
	client := multirpc.NewTcpClient(address, address)
	c.mutex.Lock()
	c.clients[address] = &client
	c.tasksHandedOut[address] = make(map[uint]task.Task)
	c.mutex.Unlock()
	c.workerWait.Add(1)
}

type getTaskResult struct {
	todo task.Task
	err  error
}

// getTaskAsync calls GetTask in the background since it blocks until a task is available or all work is done
func getTaskAsync(c *Coordinator, address string) <-chan getTaskResult {
	result := make(chan getTaskResult, 1)
	go func() {
		var todo task.Task
		err := c.GetTask(address, &todo)
		result <- getTaskResult{todo, err}
	}()
	return result
}

func waitForResult(t *testing.T, result <-chan getTaskResult) getTaskResult {
	t.Helper()
	select {
	case r := <-result:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("GetTask did not return")
		return getTaskResult{}
	}
}

func getTask(t *testing.T, c *Coordinator, address string) task.Task {
	t.Helper()
	r := waitForResult(t, getTaskAsync(c, address))
	if r.err != nil {
		t.Fatalf("GetTask(%s) returned error: %s", address, r.err)
	}
	return r.todo
}

func TestGetTaskRecordsHandedOutTask(t *testing.T) {
	c := newTestCoordinator()
	addTestWorker(c, "a")
	c.tasksTodo <- testTask(7, 1)

	todo := getTask(t, c, "a")
	if todo.ID != 7 || todo.WorkerAddress != "a" {
		t.Errorf("got task %d for worker %q, want task 7 for worker \"a\"", todo.ID, todo.WorkerAddress)
	}
	if _, ok := c.tasksHandedOut["a"][7]; !ok {
		t.Error("task 7 should be tracked as handed out to worker a")
	}
}

// Workers must not be told to stop while another worker still holds tasks that may need to be handed out again
func TestGetTaskWaitsWhileTasksAreOutstanding(t *testing.T) {
	c := newTestCoordinator()
	addTestWorker(c, "a")
	addTestWorker(c, "b")
	c.tasksTodo <- testTask(1, 1)
	getTask(t, c, "a")

	result := getTaskAsync(c, "b")
	select {
	case r := <-result:
		t.Fatalf("GetTask returned (%v, %v) while worker a still holds a task", r.todo.ID, r.err)
	case <-time.After(100 * time.Millisecond):
	}

	// Worker a leaves without returning its task, so worker b should receive it
	var nothing misc.Nothing
	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatalf("DeRegisterWorker returned error: %s", err)
	}
	r := waitForResult(t, result)
	if r.err != nil || r.todo.ID != 1 || r.todo.WorkerAddress != "b" {
		t.Errorf("worker b got (%d, %q, %v), want task 1 assigned to b", r.todo.ID, r.todo.WorkerAddress, r.err)
	}
}

func TestGetTaskReportsNoMoreTasksWhenAllDone(t *testing.T) {
	c := newTestCoordinator()
	addTestWorker(c, "a")
	result := getTaskAsync(c, "a")

	close(c.allTasksDone)
	r := waitForResult(t, result)
	if r.err == nil || r.err.Error() != task.NoMoreTasksMessage {
		t.Errorf("GetTask error = %v, want %q", r.err, task.NoMoreTasksMessage)
	}
}

func TestGetTaskFromUnregisteredWorker(t *testing.T) {
	c := newTestCoordinator()
	c.tasksTodo <- testTask(1, 1)

	r := waitForResult(t, getTaskAsync(c, "ghost"))
	if r.err == nil {
		t.Fatal("GetTask from an unregistered worker should return an error")
	}
	if len(c.tasksTodo) != 1 {
		t.Error("the queued task should not be taken by an unregistered worker")
	}
}

func TestDeRegisterWorkerRequeuesUnfinishedTasks(t *testing.T) {
	c := newTestCoordinator()
	defer close(c.allTasksDone) // releases the GetTask call left waiting at the end of the test
	addTestWorker(c, "a")
	addTestWorker(c, "b")
	c.tasksTodo <- testTask(1, 1)
	c.tasksTodo <- testTask(2, 1)
	first, second := getTask(t, c, "a"), getTask(t, c, "a")

	// Worker a finishes one task before leaving
	var nothing misc.Nothing
	if err := c.ReturnTask(rendered(first), &nothing); err != nil {
		t.Fatal(err)
	}
	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatal(err)
	}

	requeued := getTask(t, c, "b")
	if requeued.ID != second.ID {
		t.Errorf("requeued task = %d, want unfinished task %d", requeued.ID, second.ID)
	}
	result := getTaskAsync(c, "b")
	select {
	case r := <-result:
		t.Errorf("the finished task should not be requeued, but GetTask returned task %d", r.todo.ID)
	case <-time.After(100 * time.Millisecond):
	}
}

// A worker can be removed after missing a roll call and still try to deregister itself later
func TestDeRegisterWorkerTwice(t *testing.T) {
	c := newTestCoordinator()
	addTestWorker(c, "a")

	var nothing misc.Nothing
	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatalf("first DeRegisterWorker returned error: %s", err)
	}
	if err := c.DeRegisterWorker("a", &nothing); err == nil {
		t.Error("second DeRegisterWorker should return an error")
	}
	c.workerWait.Wait() // would block forever if the first call did not mark the worker done
}

func TestReturnTaskIgnoresDuplicatesAndRemovedWorkers(t *testing.T) {
	c := newTestCoordinator()
	addTestWorker(c, "a")
	addTestWorker(c, "b")
	c.tasksTodo <- testTask(1, 1)
	fromA := getTask(t, c, "a")

	// Worker a is removed, so its task goes to worker b
	var nothing misc.Nothing
	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatal(err)
	}
	fromB := getTask(t, c, "b")

	// Worker a was only slow and returns its copy late, then b returns the same task twice
	for _, done := range []task.Task{fromA, fromB, fromB} {
		if err := c.ReturnTask(rendered(done), &nothing); err != nil {
			t.Fatalf("ReturnTask returned error: %s", err)
		}
	}

	if len(c.tasksDone) != 1 {
		t.Fatalf("%d copies of task 1 were queued for ingestion, want 1", len(c.tasksDone))
	}
	if ingested := <-c.tasksDone; ingested.WorkerAddress != "b" {
		t.Errorf("ingested copy came from worker %q, want b", ingested.WorkerAddress)
	}
}

// waitForWorkers fails the test if workerWait does not reach zero, instead of hanging
func waitForWorkers(t *testing.T, c *Coordinator) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.workerWait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workerWait.Wait did not return; a registration was counted without a matching deregistration")
	}
}

// Addresses without a port make RegisterWorker's connection attempt fail immediately (it only logs a warning), so the
// real RegisterWorker can be used in tests
func TestRegisterWorkerTwice(t *testing.T) {
	c := newTestCoordinator()
	var nothing misc.Nothing
	if err := c.RegisterWorker(registration("a"), &nothing); err != nil {
		t.Fatalf("first RegisterWorker returned error: %s", err)
	}
	if err := c.RegisterWorker(registration("a"), &nothing); err == nil {
		t.Error("registering the same worker twice should return an error")
	}
	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatalf("DeRegisterWorker returned error: %s", err)
	}
	waitForWorkers(t, c)
}

// Once all tasks are done ingestTasks waits for registered workers to leave, so new workers must be turned away
func TestRegisterWorkerAfterAllTasksDone(t *testing.T) {
	c := newTestCoordinator()
	close(c.allTasksDone)

	var nothing misc.Nothing
	err := c.RegisterWorker(registration("late"), &nothing)
	if err == nil || err.Error() != task.NoMoreTasksMessage {
		t.Errorf("RegisterWorker error = %v, want %q", err, task.NoMoreTasksMessage)
	}
	if c.isRegistered("late") {
		t.Error("a worker arriving after all tasks are done should not be registered")
	}
	waitForWorkers(t, c)
}

// Workers registering and deregistering concurrently must always leave the wait group balanced
func TestRegisterAndDeRegisterConcurrently(t *testing.T) {
	c := newTestCoordinator()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			address := fmt.Sprintf("worker-%d", i)
			var nothing misc.Nothing
			if err := c.RegisterWorker(registration(address), &nothing); err != nil {
				t.Errorf("RegisterWorker(%s) returned error: %s", address, err)
				return
			}
			if err := c.DeRegisterWorker(address, &nothing); err != nil {
				t.Errorf("DeRegisterWorker(%s) returned error: %s", address, err)
			}
		}()
	}
	wg.Wait()
	waitForWorkers(t, c)
}

// testTask returns a 2x1 pixel task for the given ID and image
func testTask(id uint, imageNumber uint) task.Task {
	return task.NewTask(id, imageNumber, task.Frame{}, image.Rect(0, 0, 2, 1))
}

// rendered returns the task with correctly sized pixel data, as a worker would return it
func rendered(t task.Task) task.Task {
	t.Pixels = make([]byte, t.ResultSize())
	return t
}

func registration(address string) task.Registration {
	return task.Registration{Address: address, ProtocolVersion: task.ProtocolVersion}
}

func TestRegisterWorkerRejectsOtherProtocolVersions(t *testing.T) {
	c := newTestCoordinator()
	var nothing misc.Nothing
	err := c.RegisterWorker(task.Registration{Address: "old", ProtocolVersion: task.ProtocolVersion - 1}, &nothing)
	if err == nil {
		t.Fatal("a worker with a different protocol version should be rejected")
	}
	if c.isRegistered("old") {
		t.Error("a rejected worker should not be registered")
	}
}

// A result with the wrong amount of pixel data must not be ingested, and the task must stay assigned so it is handed
// out again when the worker leaves
func TestReturnTaskRejectsWrongSizedResult(t *testing.T) {
	c := newTestCoordinator()
	defer close(c.allTasksDone)
	addTestWorker(c, "a")
	addTestWorker(c, "b")
	c.tasksTodo <- testTask(1, 1)
	todo := getTask(t, c, "a")

	var nothing misc.Nothing
	for _, size := range []int{0, todo.ResultSize() - 1, todo.ResultSize() + 1} {
		todo.Pixels = make([]byte, size)
		if err := c.ReturnTask(todo, &nothing); err == nil {
			t.Errorf("a result with %d bytes was accepted for a task needing %d", size, todo.ResultSize())
		}
	}
	if len(c.tasksDone) != 0 {
		t.Fatalf("%d invalid results were queued for ingestion", len(c.tasksDone))
	}

	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatal(err)
	}
	if requeued := getTask(t, c, "b"); requeued.ID != todo.ID {
		t.Errorf("requeued task = %d, want %d", requeued.ID, todo.ID)
	}
}

// Only the pixel data comes from the worker, so changing other fields cannot make it paint outside its region
func TestReturnTaskUsesCoordinatorsRecordOfTheTask(t *testing.T) {
	c := newTestCoordinator()
	addTestWorker(c, "a")
	c.tasksTodo <- task.NewTask(1, 1, task.Frame{}, image.Rect(4, 2, 6, 3))
	todo := rendered(getTask(t, c, "a"))
	todo.Bounds = image.Rect(0, 0, 1, 2) // same pixel count as the real bounds
	todo.ImageNumber = 99

	var nothing misc.Nothing
	if err := c.ReturnTask(todo, &nothing); err != nil {
		t.Fatalf("ReturnTask returned error: %s", err)
	}
	ingested := <-c.tasksDone
	if ingested.Bounds != image.Rect(4, 2, 6, 3) || ingested.ImageNumber != 1 {
		t.Errorf("ingested bounds %v of image %d, want the original bounds of image 1", ingested.Bounds, ingested.ImageNumber)
	}
}

// The coordinator shuts down after exactly taskCount tasks, so the plan must match what generateTasks produces
func TestPlannedTasksMatchGeneratedTasks(t *testing.T) {
	const width, height = 37, 23
	for _, generation := range []task.Generation{task.Row, task.Column, task.Image, task.Grid} {
		t.Run(generation.String(), func(t *testing.T) {
			c := newTestCoordinator()
			c.settings.MandelbrotSettings = mandelbrot.Settings{Width: width, Height: height}
			c.settings.TaskGeneration = generation
			c.settings.TransitionSettings = []transitionSettings{
				{MagnificationStart: 1, MagnificationEnd: 4, MagnificationStep: 2}, // 3 frames
				{MagnificationStart: 4, MagnificationEnd: 1, MagnificationStep: 2}, // 3 frames
			}
			if err := c.planTasks(); err != nil {
				t.Fatalf("planTasks returned error: %s", err)
			}
			if c.imageCount != 6 {
				t.Fatalf("imageCount = %d, want 6", c.imageCount)
			}

			generated := make(chan struct{})
			go func() {
				c.generateTasks()
				close(generated)
			}()

			ids := make(map[uint]bool)
			pixelsPerImage := make(map[uint]int)
			for range c.taskCount {
				select {
				case todo := <-c.tasksTodo:
					ids[todo.ID] = true
					pixelsPerImage[todo.ImageNumber] += todo.PixelCount()
				case <-time.After(2 * time.Second):
					t.Fatalf("only %d of the %d planned tasks were generated", len(ids), c.taskCount)
				}
			}
			<-generated
			if extra := len(c.tasksTodo); extra != 0 {
				t.Fatalf("%d tasks were generated beyond the %d planned", extra, c.taskCount)
			}

			if uint(len(ids)) != c.taskCount {
				t.Errorf("got %d distinct task IDs, want %d", len(ids), c.taskCount)
			}
			for imageNumber := uint(1); imageNumber <= c.imageCount; imageNumber++ {
				if pixelsPerImage[imageNumber] != width*height {
					t.Errorf("image %d has tasks covering %d pixels, want %d", imageNumber, pixelsPerImage[imageNumber], width*height)
				}
			}
		})
	}
}

func TestPlanTasksRejectsUnknownGeneration(t *testing.T) {
	c := newTestCoordinator()
	c.settings.MandelbrotSettings = mandelbrot.Settings{Width: 10, Height: 10}
	c.settings.TaskGeneration = task.Generation(99)
	c.settings.TransitionSettings = []transitionSettings{{MagnificationStart: 1, MagnificationEnd: 2, MagnificationStep: 2}}
	if err := c.planTasks(); err == nil {
		t.Error("planTasks should reject an unknown generation type")
	}
}
