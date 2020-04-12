package main

import (
	"fmt"
	"image/color"
	"image/png"
	"log"
	"os"
)

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

// @todo update this to handle different task types?
// @todo [WIP] update this to use row/column task types
func startCoordinator() {
	Debug.Printf("Starting coordinator")

	coordinator := newCoordinator(getLocalAddress(), 10000)
	go coordinator.GenerateTasks()

	for c := 1; c <= coordinator.TaskCount; c++ {
		task := <-coordinator.TasksDone

		for it := 0; it < len(task.Iterations); it++ {
			finalColor := color.RGBA{R: 255, G: 255, B: 255, A: 0xff}
			if task.Iterations[it] == coordinator.maxIterations {
				finalColor = color.RGBA{R: 0, G: 0, B: 0, A: 0xff}
			}
			coordinator.Images[task.ImageNumber].SetRGBA(it, task.Row, finalColor)
		}
	}
	close(coordinator.TasksDone)

	Info.Print("Generating images")
	for i, image := range coordinator.Images {
		name := fmt.Sprintf("images/%d.png", i)
		f, _ := os.Create(name)
		png.Encode(f, image)
		Info.Printf("Generated image %d", i)
	}
	Info.Print("Done generating images")

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
