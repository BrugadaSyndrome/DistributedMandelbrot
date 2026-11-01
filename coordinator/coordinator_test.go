package coordinator

import (
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
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
	c.tasksTodo <- task.NewTask(7, 1)

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
	c.tasksTodo <- task.NewTask(1, 1)
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
	c.tasksTodo <- task.NewTask(1, 1)

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
	c.tasksTodo <- task.NewTask(1, 1)
	c.tasksTodo <- task.NewTask(2, 1)
	first, second := getTask(t, c, "a"), getTask(t, c, "a")

	// Worker a finishes one task before leaving
	var nothing misc.Nothing
	if err := c.ReturnTask(first, &nothing); err != nil {
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
	c.tasksTodo <- task.NewTask(1, 1)
	fromA := getTask(t, c, "a")

	// Worker a is removed, so its task goes to worker b
	var nothing misc.Nothing
	if err := c.DeRegisterWorker("a", &nothing); err != nil {
		t.Fatal(err)
	}
	fromB := getTask(t, c, "b")

	// Worker a was only slow and returns its copy late, then b returns the same task twice
	for _, done := range []task.Task{fromA, fromB, fromB} {
		if err := c.ReturnTask(done, &nothing); err != nil {
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
