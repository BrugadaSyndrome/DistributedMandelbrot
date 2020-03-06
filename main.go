package main

import (
	"log"
)

var (
	Error   *log.Logger // logLevel = 1
	Warning *log.Logger // logLevel = 2
	Info    *log.Logger // logLevel = 3
	Debug   *log.Logger // Loglevel = 4

	boundary, centerX, centerY, magnificationEnd, magnificationStart, magnificationStep float64
	height, maxIterations, width, workerCount                                           int
	ipAddress, port                                                                     string
	isWorker, isCoordinator                                                             bool
)

func main() {
	InitLogger(4)
	parseArguemnts()

	if isCoordinator {
		Info.Printf("Starting coordinator")
		// todo: check if there is already a coordinator
		// coordinator := newCoordinator(ipAddress, port)
		/*
			go coordinator.GenerateTasks()

			workers := make([]Worker, workerCount)
			for i := 0; i < workerCount; i++  {
				workers[i] = newWorker()
				// workers[i].Work()
			}

			for task := range coordinator.Tasks {
				fmt.Printf("%+v\n", task)
			}
		*/
	}

	if isWorker {
		Info.Printf("Starting worker")
		// todo: make sure the coordinator is set up to accept another worker

		// worker := newWorker(ipAddress, port)
	}

}
