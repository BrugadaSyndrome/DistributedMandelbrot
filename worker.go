package main

import (
	"fmt"
	"net/rpc"
	"time"
)

type Worker struct {
	Client    *rpc.Client
	Done      chan string
	IpAddress string
	Name      string
	Port      int
	Settings  TaskSettings
}

func newWorker(ipAddress string, port int, done chan string) Worker {
	worker := Worker{
		IpAddress: ipAddress,
		Name:      fmt.Sprintf("Worker[%s:%d]", ipAddress, port),
		Port:      port,
		Done:      done,
	}

	worker.Client = worker.connectMaster(coordinatorAddress)

	Info.Printf("RPC for node %s - %+v", worker.Name, worker)
	newRPCServer(&worker, ipAddress, port)

	return worker
}

func (w *Worker) connectMaster(masterAddress string) *rpc.Client {
	client, err := rpc.DialHTTP("tcp", masterAddress)
	if err != nil {
		Error.Fatalf("Failed dailing address: %s - %s", masterAddress, err)
	}
	Info.Printf("Opened connection to master at %s", masterAddress)
	return client
}

func (w *Worker) callMaster(method string, request interface{}, reply interface{}) error {
	maxAttempts := 3
	var err error
	for {
		// The call was a success
		err = w.Client.Call(method, request, reply)
		if err == nil {
			break
		}
		// All work is done
		if err.Error() == "all tasks handed out" {
			Info.Printf("%s - All tasks handed out", w.Name)
			break
		}

		w.Client.Close()
		Warning.Printf("%s - Unable to call master. Attempting to open connnection again", w.Name)
		w.Client = w.connectMaster(coordinatorAddress)
		maxAttempts--
		if maxAttempts <= 0 {
			Error.Printf("%s - failed to call master at address: %s, method: %s, request: %v, reply: %v, error: %v", w.Name, coordinatorAddress, method, request, reply, err)
			break
		}
	}
	return err
}

func (w *Worker) ProcessTasks() {
	var junk Nothing
	var count int
	var startTime time.Time
	var elapsedTime time.Duration

	// Fetch task settings from coordinator
	var settings TaskSettings
	err := w.callMaster("Coordinator.TaskSettings", junk, &settings)
	if err != nil {
		Error.Fatalf("%s - Failed to get task settings: %s", w.Name, err)
	}
	Info.Printf("%s - Got task settings: %+v", w.Name, settings)
	w.Settings = settings

	Info.Printf("%s - is now processing tasks", w.Name)
	startTime = time.Now()
	for {
		var task LineTask

		// Ask coordinator for a task
		err := w.callMaster("Coordinator.RequestTask", junk, &task)
		if err != nil {
			Debug.Printf(err.Error())
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
		w.callMaster("Coordinator.TaskFinished", task, &junk)
		count++
	}
	elapsedTime = time.Since(startTime)

	w.Client.Close()
	Info.Printf("%s - is done processing %d tasks in %s", w.Name, count, elapsedTime)
	w.Done <- w.Name
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

	return iteration
}
