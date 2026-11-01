package worker

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"fmt"
	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
	"sync"
	"sync/atomic"
	"time"
)

type Worker struct {
	coordinatorAddress string
	logger             bslogger.Logger
	mandelbrot         mandelbrot.Mandelbrot
	myAddress          string
	stopOnce           sync.Once
	stopped            chan struct{} // closed by shutdown so the background goroutines know to exit
	tasksCompleted     atomic.Uint64 // written by processTasks, read by the heartbeat ticker

	ServerClient multirpc.TcpServerClient
}

func newWorker(logger bslogger.Logger) *Worker {
	return &Worker{
		logger:  logger,
		stopped: make(chan struct{}),
	}
}

// NewWorker returns a pointer since the RPC server and background goroutines must share one Worker
func NewWorker(settingsFile string) *Worker {
	settings := NewSettings(settingsFile)
	worker := newWorker(bslogger.NewLogger("Worker", bslogger.Normal, nil))
	worker.coordinatorAddress = settings.CoordinatorAddress
	misc.CheckError(settings.Verify(), worker.logger, misc.Fatal)

	// Find a free port to use for this worker
	port, err := misc.GetFreePort()
	misc.CheckError(err, worker.logger, misc.Fatal)
	worker.logger.Debugf("Found free port: %d", port)
	worker.myAddress = fmt.Sprintf("%s:%d", misc.GetLocalAddress(), port)
	worker.logger = bslogger.NewLogger(fmt.Sprintf("Worker %s", worker.myAddress), bslogger.Normal, nil)
	worker.ServerClient = multirpc.NewTcpServerClient(worker, worker.myAddress, worker.myAddress, settings.CoordinatorAddress, settings.CoordinatorAddress)
	misc.CheckError(worker.ServerClient.Server.Run(), worker.logger, misc.Fatal)

	// Register with the coordinator
	misc.CheckError(worker.ServerClient.Client.Connect(), worker.logger, misc.Fatal)
	var nothing misc.Nothing
	err = worker.ServerClient.Client.Call("Coordinator.RegisterWorker", worker.myAddress, &nothing)
	if err != nil && err.Error() == task.NoMoreTasksMessage {
		// The run finished before this worker joined. Stop quietly rather than exiting the process, which would also
		// stop any other workers in it before they deregister.
		worker.logger.Info("Coordinator has no more tasks, so this worker will not start")
		worker.shutdown()
		return worker
	}
	misc.CheckError(err, worker.logger, misc.Fatal)

	// Get Mandelbrot settings from the coordinator
	var mandelbrotSettings mandelbrot.Settings
	misc.CheckError(worker.ServerClient.Client.Call("Coordinator.GetMandelbrotSettings", nothing, &mandelbrotSettings), worker.logger, misc.Fatal)
	worker.mandelbrot = mandelbrot.NewMandelbrot(mandelbrotSettings)

	go worker.tickers()
	go worker.processTasks()

	return worker
}

func (w *Worker) tickers() {
	rollCall := time.NewTicker(time.Minute)
	heartBeat := time.NewTicker(30 * time.Second)
	defer rollCall.Stop()
	defer heartBeat.Stop()

	for {
		select {
		case <-w.stopped:
			return

		case _ = <-rollCall.C:
			w.logger.Debug("Roll call ticker")
			var junk misc.Nothing
			var reply bool
			err := w.ServerClient.Client.Call("Coordinator.RollCall", junk, &reply)
			if err != nil {
				// Cannot communicate with the Coordinator so we should shut down. processTasks will then fail its next
				// call and stop as well.
				w.logger.Warningf("Coordinator missed roll call: %s", err)
				w.shutdown()
				return
			}

		case _ = <-heartBeat.C:
			w.logger.Debug("Heart beat ticker")
			w.logger.Info(w.status())
		}
	}
}

func (w *Worker) processTasks() {
	w.logger.Info("Processing tasks")

	var nothing misc.Nothing
	var elapsedTime time.Duration
	var startTime = time.Now()

	for {
		var taskTodo task.Task
		var err error

		err = w.ServerClient.Client.Call("Coordinator.GetTask", w.myAddress, &taskTodo)
		if err != nil {
			// This is an expected error. No more work to do
			if err.Error() == task.NoMoreTasksMessage {
				break
			}
			// Stop only this worker; other workers in the same process may still be able to keep going
			w.logger.Errorf("Unable to get a task: %s", err.Error())
			break
		}

		for {
			// Process each coordinate given
			coordinate, err := taskTodo.GetNextTask()
			if err != nil {
				break
			}

			points := w.mandelbrot.GetPointsToCalculate(coordinate)
			iterations := w.mandelbrot.EscapeTimeMultiple(points)
			color := w.mandelbrot.GetColorMultiple(iterations)

			pixel := task.Pixel{
				Color:  color,
				Column: coordinate.Column,
				Row:    coordinate.Row,
			}
			taskTodo.AddResult(pixel)
		}

		err = w.ServerClient.Client.Call("Coordinator.ReturnTask", taskTodo, &nothing)
		if err != nil {
			w.logger.Errorf("Unable to return a task: %s", err.Error())
			break
		}
		w.tasksCompleted.Add(1)
	}

	elapsedTime = time.Since(startTime)

	w.logger.Info("Done processing tasks")
	w.logger.Debugf("Processed %d tasks in %s", w.tasksCompleted.Load(), elapsedTime)

	w.logger.Info("Shutting down")
	misc.CheckError(w.ServerClient.Client.Call("Coordinator.DeRegisterWorker", w.myAddress, &nothing), w.logger, misc.Warning)
	w.shutdown()
}

// shutdown disconnects from the coordinator and stops this worker's RPC server. Both the roll call ticker and
// processTasks can call it, but it only runs once since stopping the multirpc server a second time panics.
func (w *Worker) shutdown() {
	w.stopOnce.Do(func() {
		close(w.stopped)
		misc.CheckError(w.ServerClient.Client.Disconnect(), w.logger, misc.Warning)
		misc.CheckError(w.ServerClient.Server.Stop(), w.logger, misc.Warning)
	})
}

func (w *Worker) RollCall(request misc.Nothing, reply *bool) error {
	*reply = true
	return nil
}

// status summarizes progress for the heartbeat log and is safe to call from any goroutine
func (w *Worker) status() string {
	return fmt.Sprintf("Tasks [Completed: %d]", w.tasksCompleted.Load())
}
