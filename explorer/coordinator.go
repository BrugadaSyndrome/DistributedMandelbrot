package explorer

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
)

// tileResult is a computed tile on its way to a render request, its samples compressed with compressSamples
type tileResult struct {
	ID         tileID
	Compressed []byte
	Cached     bool // true when it came from the cache rather than a worker
}

// job is one render request's interest in tiles. results has room for every tile the job waits for, so delivering a
// result never blocks.
type job struct {
	results chan tileResult
}

func newJob(tileCount int) *job {
	return &job{results: make(chan tileResult, tileCount)}
}

// Coordinator hands explorer tiles to workers. It is named Coordinator because net/rpc registers methods under the bare
// type name and workers call "Coordinator.GetTask" and so on; it follows the same worker rules as the movie
// coordinator, but its work never ends and queued tiles can be dropped when nobody wants them any more.
//
// Concurrency: everything below mutex is guarded by it. Fields above it are set before the RPC server starts.
type Coordinator struct {
	logger     bslogger.Logger
	onResult   func(id tileID, compressed []byte) // called with every tile a worker computes, to cache it
	renderWith mandelbrot.Settings                // sent to workers that ask for settings; tiles carry their own

	mutex          sync.Mutex
	clients        map[string]*multirpc.TcpClient
	inProgress     map[tileID]uint               // tiles handed out, by how many workers hold them
	nextTaskID     uint                          // task IDs only need to be unique per worker, but a counter is simplest
	queue          []tileID                      // tiles waiting for a worker, most wanted first
	tasksHandedOut map[string]map[uint]task.Task // keep track of all tasks workers have
	tileOfTask     map[string]map[uint]tileID    // the tile behind each handed out task
	waiting        map[tileID]map[*job]bool      // jobs waiting for each queued or in progress tile
	wake           chan struct{}                 // signalled when the queue gains tiles; holds at most one signal

	Server multirpc.TcpServer
}

func newCoordinator(logger bslogger.Logger, renderWith mandelbrot.Settings, onResult func(tileID, []byte)) *Coordinator {
	return &Coordinator{
		clients:        make(map[string]*multirpc.TcpClient),
		inProgress:     make(map[tileID]uint),
		logger:         logger,
		onResult:       onResult,
		renderWith:     renderWith,
		tasksHandedOut: make(map[string]map[uint]task.Task),
		tileOfTask:     make(map[string]map[uint]tileID),
		waiting:        make(map[tileID]map[*job]bool),
		wake:           make(chan struct{}, 1),
	}
}

// start begins serving workers. Like the movie coordinator's server, it must start last.
func (c *Coordinator) start(address string) {
	c.Server = multirpc.NewTcpServer(c, address, "ExplorerServer")
	misc.CheckError(c.Server.Run(), c.logger, misc.Fatal)
	go c.tickers()
}

func (c *Coordinator) tickers() {
	rollCall := time.NewTicker(time.Minute)
	heartBeat := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-rollCall.C:
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
				if err := v.Call("Worker.RollCall", junk, &reply); err != nil {
					c.logger.Warningf("Worker %s missed roll call: %s", v.Name(), err)
					var nothing misc.Nothing
					misc.CheckError(c.DeRegisterWorker(v.Name(), &nothing), c.logger, misc.Warning)
				}
			}

		case <-heartBeat.C:
			c.logger.Info(c.status())
		}
	}
}

// status summarizes the workers and tiles for the heartbeat log
func (c *Coordinator) status() string {
	workers, queued, inProgress := c.counts()
	return fmt.Sprintf("Workers: %d Tiles [Queued: %d In progress: %d]", workers, queued, inProgress)
}

// counts reports how many workers are registered and how many tiles are queued and being computed
func (c *Coordinator) counts() (workers int, queued int, inProgress int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.clients), len(c.queue), len(c.inProgress)
}

// submit makes j wait for ids, which the cache did not have. Tiles nobody is working on yet go to the front of the
// queue in the order given, ahead of tiles that earlier jobs still want; a tile that is already queued or being
// computed is only waited for, so it is never computed twice at once.
func (c *Coordinator) submit(j *job, ids []tileID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	queued := make(map[tileID]bool, len(c.queue))
	for _, id := range c.queue {
		queued[id] = true
	}

	front := make([]tileID, 0, len(ids))
	for _, id := range ids {
		if c.waiting[id] == nil {
			c.waiting[id] = make(map[*job]bool)
		}
		if c.waiting[id][j] {
			continue // asked for twice
		}
		c.waiting[id][j] = true
		if c.inProgress[id] == 0 {
			front = append(front, id)
			delete(queued, id)
		}
	}
	// What is left in queued is wanted by earlier jobs only, and keeps its order behind the new tiles
	rest := slices.DeleteFunc(c.queue, func(id tileID) bool { return !queued[id] })
	c.queue = append(front, rest...)
	c.signal()
}

// cancel stops j waiting for its tiles. Queued tiles no other job wants are dropped; tiles already handed out are
// still finished and cached.
func (c *Coordinator) cancel(j *job) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for id, jobs := range c.waiting {
		if jobs[j] {
			delete(jobs, j)
			if len(jobs) == 0 {
				delete(c.waiting, id)
			}
		}
	}
	c.queue = slices.DeleteFunc(c.queue, func(id tileID) bool { return len(c.waiting[id]) == 0 })
}

// signal wakes one waiting GetTask if there is work. Must hold mutex.
func (c *Coordinator) signal() {
	if len(c.queue) == 0 {
		return
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Coordinator) RegisterWorker(registration task.Registration, reply *misc.Nothing) error {
	workerServerAddress := registration.Address
	if registration.ProtocolVersion != task.ProtocolVersion {
		return fmt.Errorf("worker %s uses protocol version %d but the explorer uses version %d; run the same build on both",
			workerServerAddress, registration.ProtocolVersion, task.ProtocolVersion)
	}

	client := multirpc.NewTcpClient(workerServerAddress, workerServerAddress)
	c.mutex.Lock()
	if _, registered := c.clients[workerServerAddress]; registered {
		c.mutex.Unlock()
		return fmt.Errorf("worker %s is already registered", workerServerAddress)
	}
	c.clients[workerServerAddress] = &client
	c.tasksHandedOut[workerServerAddress] = make(map[uint]task.Task)
	c.tileOfTask[workerServerAddress] = make(map[uint]tileID)
	c.mutex.Unlock()
	misc.CheckError(client.Connect(), c.logger, misc.Warning)

	c.logger.Infof("Worker joined: %s", workerServerAddress)
	return nil
}

func (c *Coordinator) DeRegisterWorker(workerServerAddress string, reply *misc.Nothing) error {
	c.mutex.Lock()
	client, registered := c.clients[workerServerAddress]
	if !registered {
		c.mutex.Unlock()
		return fmt.Errorf("worker %s is not registered", workerServerAddress)
	}
	// Tiles this worker had go back to the front of the queue if a job still wants them
	unfinished := 0
	for _, id := range c.tileOfTask[workerServerAddress] {
		c.finished(id)
		if len(c.waiting[id]) > 0 && c.inProgress[id] == 0 && !slices.Contains(c.queue, id) {
			c.queue = append([]tileID{id}, c.queue...)
			unfinished++
		}
	}
	delete(c.clients, workerServerAddress)
	delete(c.tasksHandedOut, workerServerAddress)
	delete(c.tileOfTask, workerServerAddress)
	c.signal()
	c.mutex.Unlock()

	misc.CheckError(client.Disconnect(), c.logger, misc.Warning)
	c.logger.Infof("Worker left: %s (%d unfinished tiles requeued)", workerServerAddress, unfinished)
	return nil
}

// finished records that one worker no longer holds id. Must hold mutex.
func (c *Coordinator) finished(id tileID) {
	if c.inProgress[id] <= 1 {
		delete(c.inProgress, id)
	} else {
		c.inProgress[id]--
	}
}

func (c *Coordinator) RollCall(nothing misc.Nothing, present *bool) error {
	*present = true
	return nil
}

// GetTask hands the worker the next queued tile, waiting as long as it takes for one. The explorer's work never ends,
// so unlike the movie coordinator it never answers with task.NoMoreTasksMessage.
func (c *Coordinator) GetTask(workerAddress string, assigned *task.Task) error {
	for {
		c.mutex.Lock()
		handedOut, registered := c.tasksHandedOut[workerAddress]
		if !registered {
			// e.g. dropped by roll call while waiting; pass on any wake up it took so another worker gets the tile
			c.signal()
			c.mutex.Unlock()
			return fmt.Errorf("worker %s is not registered", workerAddress)
		}
		if len(c.queue) > 0 {
			id := c.queue[0]
			c.queue = c.queue[1:]
			c.nextTaskID++
			todo := id.task(c.nextTaskID)
			todo.WorkerAddress = workerAddress
			handedOut[todo.ID] = todo
			c.tileOfTask[workerAddress][todo.ID] = id
			c.inProgress[id]++
			// Pass the wake up along so other waiting workers take the rest of the queue
			c.signal()
			c.mutex.Unlock()
			*assigned = todo
			return nil
		}
		c.mutex.Unlock()
		<-c.wake
	}
}

// ReturnTask accepts a finished tile only if it is still handed out to the worker returning it, and only takes the
// samples from it: the tile's identity comes from the coordinator's own record. A result of the wrong size is rejected
// and the tile stays assigned, so it is requeued when the worker leaves.
func (c *Coordinator) ReturnTask(done task.Task, nothing *misc.Nothing) error {
	c.mutex.Lock()
	assigned, handedOut := c.tasksHandedOut[done.WorkerAddress][done.ID]
	if !handedOut {
		c.mutex.Unlock()
		c.logger.Warningf("Ignoring task %d from worker %s since it is no longer assigned to that worker", done.ID, done.WorkerAddress)
		return nil
	}
	assigned.Pixels = done.Pixels
	if !assigned.HasValidResult() {
		c.mutex.Unlock()
		return fmt.Errorf("task %d returned %d bytes of samples but needs %d", done.ID, len(done.Pixels), assigned.ResultSize())
	}
	id := c.tileOfTask[done.WorkerAddress][done.ID]
	delete(c.tasksHandedOut[done.WorkerAddress], done.ID)
	delete(c.tileOfTask[done.WorkerAddress], done.ID)
	c.finished(id)
	jobs := c.waiting[id]
	delete(c.waiting, id)
	c.mutex.Unlock()

	compressed, err := compressSamples(done.Pixels)
	if err != nil {
		return fmt.Errorf("unable to compress tile %s: %w", id.tileKey, err)
	}
	for j := range jobs {
		j.results <- tileResult{ID: id, Compressed: compressed}
	}
	if c.onResult != nil {
		c.onResult(id, compressed)
	}
	return nil
}

func (c *Coordinator) GetMandelbrotSettings(nothing misc.Nothing, settings *mandelbrot.Settings) error {
	*settings = c.renderWith
	return nil
}
