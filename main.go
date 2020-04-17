package main

import (
	"fmt"
	"image/png"
	"log"
	"os"
)

// todo: move loggers into coordinator and workers
var (
	Error   *log.Logger // logLevel = 1
	Warning *log.Logger // logLevel = 2
	Info    *log.Logger // logLevel = 3
	Debug   *log.Logger // Loglevel = 4

	boundary, centerX, centerY, magnificationEnd, magnificationStart, magnificationStep float64
	height, maxIterations, width, workerCount                                           int
	coordinatorAddress                                                                  string
	isWorker, isCoordinator                                                             bool
)

func main() {
	InitLogger(3)
	parseArguemnts()

	if isCoordinator {
		startCoordinator()
	}

	if isWorker {
		startWorker()
	}
}

// todo: Figure out why in some cases images will not be generated (Maybe not all tasks are actually returned...)
// todo: Figure out a way to have colors be specified in a file
// todo: switch colors to hsv/hsl from rgb
func startCoordinator() {
	Debug.Printf("Starting coordinator")

	coordinator := newCoordinator(getLocalAddress(), 10000)
	go coordinator.GenerateTasks()

	for c := 1; c <= coordinator.TaskCount; c++ {
		task := <-coordinator.TasksDone

		for it := 0; it < len(task.Iterations); it++ {
			coordinator.ImageTasks[task.ImageNumber].Image.SetRGBA(it, task.Row, coordinator.GetColor(task.Iterations[it]))
			coordinator.ImageTasks[task.ImageNumber].PixelsLeft--
			if coordinator.ImageTasks[task.ImageNumber].PixelsLeft == 0 {
				name := fmt.Sprintf("images/%d.png", task.ImageNumber)
				f, _ := os.Create(name)
				png.Encode(f, coordinator.ImageTasks[task.ImageNumber].Image)
				Info.Printf("Generated image %d", task.ImageNumber)
				coordinator.ImageTasks[task.ImageNumber].Generated = true
			}
		}
	}
	close(coordinator.TasksDone)
	Info.Printf("%s - Done generating images", coordinator.Name)

	Info.Printf("%s - Shutting down", coordinator.Name)
}

func startWorker() {
	workerDone := make(chan string, workerCount)

	for i := 0; i < workerCount; i++ {
		worker := newWorker(getLocalAddress(), 10001+i, workerDone)

		go worker.ProcessTasks()
	}

	for {
		if workerCount <= 0 {
			break
		}
		name := <-workerDone
		workerCount--
		Info.Printf("%s - shutting down", name)
	}
}
