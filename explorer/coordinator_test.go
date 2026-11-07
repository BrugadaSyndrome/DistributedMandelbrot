package explorer

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
)

// resultRecorder collects the tiles a coordinator passes to onResult
type resultRecorder struct {
	mutex sync.Mutex
	ids   []tileID
}

func (r *resultRecorder) record(id tileID, compressed []byte) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ids = append(r.ids, id)
}

func (r *resultRecorder) recorded() []tileID {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return slices.Clone(r.ids)
}

// newTestCoordinator builds a Coordinator without starting its RPC server or tickers
func newTestCoordinator(t *testing.T) (*Coordinator, *resultRecorder) {
	t.Helper()
	recorder := &resultRecorder{}
	c := newCoordinator(bslogger.NewLogger("ExplorerTest", bslogger.Normal, nil), mandelbrot.Settings{MaxIterations: 77}, recorder.record)
	return c, recorder
}

// addTestWorker registers a worker directly, since RegisterWorker would try to connect back to it
func addTestWorker(c *Coordinator, address string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	client := multirpc.NewTcpClient(address, address)
	c.clients[address] = &client
	c.tasksHandedOut[address] = make(map[uint]task.Task)
	c.tileOfTask[address] = make(map[uint]tileID)
}

func testID(level int, x int64, y int64) tileID {
	return tileID{tileKey: tileKey{level, x, y}, Boundary: 1000, MaxIterations: 100, SuperSampling: 1}
}

// getTask calls GetTask and fails the test if it does not return in time
func getTask(t *testing.T, c *Coordinator, worker string) task.Task {
	t.Helper()
	type reply struct {
		todo task.Task
		err  error
	}
	done := make(chan reply, 1)
	go func() {
		var todo task.Task
		err := c.GetTask(worker, &todo)
		done <- reply{todo, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("GetTask: %s", r.err)
		}
		return r.todo
	case <-time.After(2 * time.Second):
		t.Fatal("GetTask did not return")
		return task.Task{}
	}
}

// tileOf is the tile a handed out task computes
func tileOf(c *Coordinator, todo task.Task) tileID {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.tileOfTask[todo.WorkerAddress][todo.ID]
}

// complete renders a task with the right amount of data and returns it
func complete(t *testing.T, c *Coordinator, todo task.Task) {
	t.Helper()
	todo.Pixels = make([]byte, todo.ResultSize())
	var nothing misc.Nothing
	if err := c.ReturnTask(todo, &nothing); err != nil {
		t.Fatalf("ReturnTask: %s", err)
	}
}

func queueOf(c *Coordinator) []tileID {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return slices.Clone(c.queue)
}

func TestGetTaskWaitsForWork(t *testing.T) {
	c, _ := newTestCoordinator(t)
	addTestWorker(c, "w1")

	got := make(chan task.Task, 1)
	go func() {
		var todo task.Task
		if err := c.GetTask("w1", &todo); err == nil {
			got <- todo
		}
	}()
	select {
	case <-got:
		t.Fatal("GetTask returned with nothing queued")
	case <-time.After(50 * time.Millisecond):
	}

	id := testID(3, -4, 1)
	c.submit(newJob(1), []tileID{id})
	select {
	case todo := <-got:
		want := id.task(todo.ID)
		if todo.Tile == nil || *todo.Tile != *want.Tile || todo.Bounds != want.Bounds || todo.WorkerAddress != "w1" {
			t.Errorf("GetTask gave %+v with tile %+v, want the task for %s", todo, todo.Tile, id.tileKey)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetTask did not return after a tile was queued")
	}
}

func TestGetTaskUnknownWorker(t *testing.T) {
	c, _ := newTestCoordinator(t)
	var todo task.Task
	if err := c.GetTask("nobody", &todo); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("GetTask for an unknown worker returned %v, want a not registered error", err)
	}
}

// Every waiting worker must get a tile, not only the one the first wake up reaches
func TestManyWaitingWorkersAllGetTiles(t *testing.T) {
	c, _ := newTestCoordinator(t)
	workers := []string{"w1", "w2", "w3", "w4"}
	for _, w := range workers {
		addTestWorker(c, w)
	}
	got := make(chan tileID, len(workers))
	for _, w := range workers {
		go func() {
			var todo task.Task
			if c.GetTask(w, &todo) == nil {
				got <- tileOf(c, todo)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond) // let them all wait
	ids := []tileID{testID(2, 0, 0), testID(2, 1, 0), testID(2, 0, 1), testID(2, 1, 1)}
	c.submit(newJob(len(ids)), ids)

	var handed []tileID
	for range workers {
		select {
		case id := <-got:
			handed = append(handed, id)
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d waiting workers got a tile", len(handed), len(workers))
		}
	}
	for _, id := range ids {
		if !slices.Contains(handed, id) {
			t.Errorf("tile %s was never handed out", id.tileKey)
		}
	}
}

func TestReturnTaskDeliversAndRecords(t *testing.T) {
	c, recorder := newTestCoordinator(t)
	addTestWorker(c, "w1")
	id := testID(1, 0, 0)
	j := newJob(1)
	c.submit(j, []tileID{id})

	todo := getTask(t, c, "w1")
	samples := make([]float32, task.TileSize*task.TileSize)
	for i := range samples {
		samples[i] = float32(i % 97)
	}
	todo.Pixels = task.EncodeSamples(samples)
	var nothing misc.Nothing
	if err := c.ReturnTask(todo, &nothing); err != nil {
		t.Fatalf("ReturnTask: %s", err)
	}

	select {
	case result := <-j.results:
		if result.ID != id || result.Cached {
			t.Errorf("job got %+v, want a computed result for %s", result.ID, id.tileKey)
		}
		decompressed, err := decompressSamples(result.Compressed)
		if err != nil || !slices.Equal(task.DecodeSamples(decompressed), samples) {
			t.Errorf("the job's samples do not match what the worker returned (error %v)", err)
		}
	default:
		t.Fatal("the waiting job got no result")
	}
	if got := recorder.recorded(); len(got) != 1 || got[0] != id {
		t.Errorf("onResult got %v, want only %s", got, id.tileKey)
	}
	if _, _, inProgress := c.counts(); inProgress != 0 {
		t.Errorf("%d tiles still in progress after the only one was returned", inProgress)
	}
}

func TestReturnTaskRejections(t *testing.T) {
	c, recorder := newTestCoordinator(t)
	addTestWorker(c, "w1")
	addTestWorker(c, "w2")
	c.submit(newJob(1), []tileID{testID(1, 0, 0)})
	todo := getTask(t, c, "w1")
	var nothing misc.Nothing

	t.Run("wrong size is an error and the tile stays assigned", func(t *testing.T) {
		short := todo
		short.Pixels = make([]byte, todo.ResultSize()-4)
		if err := c.ReturnTask(short, &nothing); err == nil {
			t.Fatal("a short result was accepted")
		}
		if _, _, inProgress := c.counts(); inProgress != 1 {
			t.Errorf("in progress = %d after a rejected result, want 1", inProgress)
		}
	})

	t.Run("from a worker it was not given to", func(t *testing.T) {
		other := todo
		other.WorkerAddress = "w2"
		other.Pixels = make([]byte, todo.ResultSize())
		if err := c.ReturnTask(other, &nothing); err != nil {
			t.Fatalf("ReturnTask: %s", err)
		}
		if len(recorder.recorded()) != 0 {
			t.Error("a result from the wrong worker was accepted")
		}
	})

	t.Run("returned twice", func(t *testing.T) {
		complete(t, c, todo)
		complete(t, c, todo)
		if got := recorder.recorded(); len(got) != 1 {
			t.Errorf("onResult called %d times for one tile returned twice, want 1", len(got))
		}
	})
}

func TestNewJobGoesFirstAndCancelDropsUnwantedTiles(t *testing.T) {
	c, _ := newTestCoordinator(t)
	a, b, shared, d := testID(4, 0, 0), testID(4, 1, 0), testID(4, 2, 0), testID(4, 3, 0)

	first := newJob(3)
	c.submit(first, []tileID{a, b, shared})
	second := newJob(2)
	c.submit(second, []tileID{d, shared})
	if got, want := queueOf(c), []tileID{d, shared, a, b}; !slices.Equal(got, want) {
		t.Fatalf("queue = %v, want the new job's tiles first: %v", got, want)
	}

	c.cancel(first)
	if got, want := queueOf(c), []tileID{d, shared}; !slices.Equal(got, want) {
		t.Errorf("queue after cancelling the first job = %v, want %v", got, want)
	}
	c.cancel(second)
	if got := queueOf(c); len(got) != 0 {
		t.Errorf("queue after cancelling every job = %v, want empty", got)
	}
}

func TestTileInProgressIsNotQueuedAgain(t *testing.T) {
	c, _ := newTestCoordinator(t)
	addTestWorker(c, "w1")
	id := testID(5, 1, 1)
	first := newJob(1)
	c.submit(first, []tileID{id})
	todo := getTask(t, c, "w1")

	second := newJob(1)
	c.submit(second, []tileID{id})
	if got := queueOf(c); len(got) != 0 {
		t.Fatalf("queue = %v, want the tile in progress not queued again", got)
	}
	// The same tile asked for twice by one job is queued once
	c.submit(newJob(2), []tileID{testID(5, 2, 2), testID(5, 2, 2)})
	if got := queueOf(c); len(got) != 1 {
		t.Errorf("queue = %v, want one entry for a repeated tile", got)
	}

	complete(t, c, todo)
	for name, j := range map[string]*job{"first": first, "second": second} {
		select {
		case <-j.results:
		default:
			t.Errorf("the %s job did not get the shared tile", name)
		}
	}
}

func TestCancelledTileInProgressIsStillCached(t *testing.T) {
	c, recorder := newTestCoordinator(t)
	addTestWorker(c, "w1")
	id := testID(2, 1, 1)
	j := newJob(1)
	c.submit(j, []tileID{id})
	todo := getTask(t, c, "w1")
	c.cancel(j)
	complete(t, c, todo)
	if got := recorder.recorded(); len(got) != 1 || got[0] != id {
		t.Errorf("onResult got %v, want the finished tile cached even though nobody waits for it", got)
	}
	select {
	case <-j.results:
		t.Error("a cancelled job still got a result")
	default:
	}
}

func TestDeRegisterRequeuesWantedTiles(t *testing.T) {
	c, _ := newTestCoordinator(t)
	addTestWorker(c, "w1")
	addTestWorker(c, "w2")
	wanted, unwanted, queued := testID(3, 0, 0), testID(3, 1, 0), testID(3, 2, 0)

	stays := newJob(2)
	c.submit(stays, []tileID{wanted, queued})
	goes := newJob(1)
	c.submit(goes, []tileID{unwanted})
	// Hand out unwanted and wanted to w1, leaving queued in the queue
	held := []tileID{tileOf(c, getTask(t, c, "w1")), tileOf(c, getTask(t, c, "w1"))}
	if !slices.Contains(held, wanted) || !slices.Contains(held, unwanted) {
		t.Fatalf("w1 holds %v, want %s and %s", held, wanted.tileKey, unwanted.tileKey)
	}
	c.cancel(goes)

	var nothing misc.Nothing
	if err := c.DeRegisterWorker("w1", &nothing); err != nil {
		t.Fatalf("DeRegisterWorker: %s", err)
	}
	if got, want := queueOf(c), []tileID{wanted, queued}; !slices.Equal(got, want) {
		t.Errorf("queue = %v, want the wanted tile back at the front: %v", got, want)
	}
	if todo := getTask(t, c, "w2"); tileOf(c, todo) != wanted {
		t.Errorf("w2 got %s, want the requeued %s", tileOf(c, todo).tileKey, wanted.tileKey)
	}
	if err := c.DeRegisterWorker("w1", &nothing); err == nil {
		t.Error("deregistering twice should fail")
	}
}

func TestRegisterWorkerChecksProtocol(t *testing.T) {
	c, _ := newTestCoordinator(t)
	var nothing misc.Nothing
	err := c.RegisterWorker(task.Registration{Address: "127.0.0.1:1", ProtocolVersion: task.ProtocolVersion - 1}, &nothing)
	if err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Errorf("RegisterWorker with an old protocol returned %v, want a protocol version error", err)
	}
}

func TestGetMandelbrotSettings(t *testing.T) {
	c, _ := newTestCoordinator(t)
	var s mandelbrot.Settings
	if err := c.GetMandelbrotSettings(misc.Nothing{}, &s); err != nil || s.MaxIterations != 77 {
		t.Errorf("GetMandelbrotSettings = %d, %v; want the explorer's defaults", s.MaxIterations, err)
	}
}
