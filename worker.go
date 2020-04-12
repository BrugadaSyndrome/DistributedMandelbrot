package main

import (
	"fmt"
	"net/rpc"
	"time"
)

type Worker struct {
	boundary      float64
	centerX       float64
	centerY       float64
	height        int
	maxIterations int
	shorterSide   int
	width         int

	Client    *rpc.Client
	IpAddress string
	Name      string
	Port      int

	Done chan string
}

func newWorker(ipAddress string, port int, done chan string) Worker {
	if width <= 0 {
		Error.Fatal("Width must be greater than 0")
	}
	if height <= 0 {
		Error.Fatal("Height must be greater than 0")
	}

	shorterSide := height
	if width < height {
		shorterSide = width
	}

	worker := Worker{
		boundary:      boundary,
		centerX:       centerX,
		centerY:       centerY,
		height:        height,
		maxIterations: maxIterations,
		shorterSide:   shorterSide,
		width:         width,

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
		err = w.Client.Call(method, request, reply)
		if err == nil {
			break
		}
		if err.Error() == "all tasks handed out" {
			Info.Printf("%s - All tasks handed out", w.Name)
			break
		}

		w.Client.Close()
		Warning.Printf("Unable to call master. Attempting to open connnection again")
		w.Client = w.connectMaster(coordinatorAddress)
		maxAttempts--
		if maxAttempts <= 0 {
			Error.Printf("Failed to call master at address: %s, method: %s, request: %v, reply: %v, error: %v", coordinatorAddress, method, request, reply, err)
			break
		}
	}
	return err
}

// @todo refactor tasks so that the master passes the centerX, centerY and magnification values first before processing any tasks
//  	 - this eliminates the need for the worker script to be passed these variables and also the overhead in the task struct itself
func (w *Worker) ProcessTasks() {
	Info.Printf("%s - is now processing tasks", w.Name)
	var junk Nothing
	var count int
	var startTime time.Time
	var elapsedTime time.Duration

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
	x := w.centerX + (float64(column)-float64(w.width)/2)/(magnification*(float64(w.shorterSide)-1))
	y := w.centerY + (float64(row)-float64(w.height)/2)/(magnification*(float64(w.shorterSide)-1))

	a, b, r, i, z := x, y, 0.0, 0.0, 0.0
	iteration := 0
	for (r+i) <= w.boundary && iteration < w.maxIterations {
		x := r - i + a
		y := z - r - i + b
		r = x * x
		i = y * y
		z = (x + y) * (x + y)
		iteration++
	}

	return iteration
}
