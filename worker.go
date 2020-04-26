package main

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type Worker struct {
	Client    *rpc.Client
	IpAddress string
	Logger    *log.Logger
	Port      int
	Settings  TaskSettings
	Wait      *sync.WaitGroup
}

func newWorker(ipAddress string, port int, wg *sync.WaitGroup) Worker {
	worker := Worker{
		IpAddress: ipAddress,
		Logger:    log.New(os.Stdout, fmt.Sprintf("Worker[%s:%d] ", ipAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		Port:      port,
		Wait:      wg,
	}

	worker.Client = worker.connectCoordinator(coordinatorAddress)

	newRPCServer(&worker, ipAddress, port)

	return worker
}

func (w *Worker) connectCoordinator(masterAddress string) *rpc.Client {
	client, err := rpc.DialHTTP("tcp", masterAddress)
	if err != nil {
		w.Logger.Fatalf("Failed dailing address: %s - %s", masterAddress, err)
	}
	w.Logger.Printf("Opened connection to coordinator at %s", masterAddress)
	return client
}

func (w *Worker) callCoordinator(method string, request interface{}, reply interface{}) error {
	err := w.Client.Call(method, request, reply)

	// The call was a success
	if err == nil {
		return nil
	}

	// All work is done
	if err.Error() == "all tasks handed out" {
		w.Logger.Print("All tasks handed out")
		return err
	}

	// Unable to make the call
	w.Client.Close()
	w.Logger.Printf("ERROR - Failed to call coordinator at address: %s, method: %s, error: %v", coordinatorAddress, method, err)
	return err
}

func (w *Worker) ProcessTasks() {
	var junk Nothing
	var count int
	var startTime time.Time
	var elapsedTime time.Duration

	// Register worker with coordinator
	address := fmt.Sprintf("%s:%d", w.IpAddress, w.Port)
	err := w.callCoordinator("Coordinator.RegisterWorker", address, &junk)
	if err != nil {
		w.Logger.Fatalf("Failed to register with coordinator: %s", err)
	}

	// Fetch task settings from coordinator
	var settings TaskSettings
	err = w.callCoordinator("Coordinator.GetTaskSettings", junk, &settings)
	if err != nil {
		w.Logger.Fatalf("Failed to get task settings: %s", err)
	}
	w.Logger.Printf("Got task settings from coordinator: %+v", settings)
	w.Settings = settings

	w.Logger.Printf("Now processing tasks")
	startTime = time.Now()
	for {
		var task LineTask

		// Ask coordinator for a task
		err := w.callCoordinator("Coordinator.RequestTask", junk, &task)
		if err != nil {
			break
		}

		// Calculate escape value
		for {
			row, column, magnification := task.NextTask()
			if row == -1 && column == -1 && magnification == -1 {
				break
			}
			task.RecordIteration(w.mandel(row, column, magnification))
		}

		// Return result to master
		err = w.callCoordinator("Coordinator.TaskFinished", task, &junk)
		if err != nil {
			w.Logger.Printf("WARNING - Coordinator.TaskFinished - %s", err)
		}
		count++
	}
	// Worker is done processing
	elapsedTime = time.Since(startTime)
	w.Logger.Printf("Done processing %d tasks in %s", count, elapsedTime)

	// Inform coordinator we are leaving and shutdown
	w.Logger.Print("Shutting down")
	w.callCoordinator("Coordinator.DeRegisterWorker", address, &junk)
	w.Client.Close()
	w.Wait.Done()
}

func (w *Worker) mandel(row int, column int, magnification float64) int {
	// Since each pixel is from [0, height] and [0, width] and not on the real axis we need to convert
	// the (column, row) point on the image to the (x, y) point in the real axis
	x := w.Settings.CenterX + (float64(column)-float64(w.Settings.Width)/2)/(magnification*(float64(w.Settings.ShorterSide)-1))
	y := w.Settings.CenterY + (float64(row)-float64(w.Settings.Height)/2)/(magnification*(float64(w.Settings.ShorterSide)-1))

	a, b, r, i, z := x, y, 0.0, 0.0, 0.0
	iteration := 0
	for (r+i) <= w.Settings.Boundary && iteration < w.Settings.MaxIterations {
		x := r - i + a
		y := z - r - i + b
		r = x * x
		i = y * y
		z = (x + y) * (x + y)
		iteration++
	}

	// When smooth coloring, avoid potential floating point issues
	// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set
	/*
		if w.Settings.SmoothColoring && iteration < w.Settings.MaxIterations {
			zn := math.Log(x*x + y*y) / 2
			nu := math.Log(zn / math.Log(2)) / math.Log(2)
			iteration = int(math.Floor(float64(iteration + 1) - nu))
		}
	*/

	return iteration
}
