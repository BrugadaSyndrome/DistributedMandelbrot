package main

import (
	"fmt"
	"image/png"
	"os"
)

/**
 * TODO
 *
 * Color
 * todo: switch colors from rgb to hsv/hsl
 * todo: option to switch between palettes and smooth coloring
 * Cache iteration results in db
 * todo: get distributed mandelbrot working inside of a multi-machine vagrant instance
 *     : including firewall stuff (avoid private network options because that wont be available normally)
 * todo: stashing results in mysql db
 * Zoom
 * todo: improve zoom by allowing 'sliding' zooms from (x0, y0) => (x1, y1)
 */

var (
	boundary, centerX, centerY, magnificationEnd, magnificationStart, magnificationStep float64
	height, maxIterations, width, workerCount                                           int
	coordinatorAddress, paletteFile                                                     string
	isWorker, isCoordinator                                                             bool
)

func main() {
	parseArguemnts()

	if isCoordinator {
		startCoordinator()
	}

	if isWorker {
		startWorker()
	}
}

// todo: switch colors from rgb to hsv/hsl
// todo: option to switch between palettes and smooth coloring

// todo: work on getting this working inside of a multi-machine vagrant instance
// todo: stashing results in mysql db

// todo: improve zoom by allowing 'sliding' zooms from (x0, y0) => (x1, y1)
func startCoordinator() {
	coordinator := newCoordinator(getLocalAddress(), 10000)
	coordinator.Logger.Print("Starting coordinator")

	coordinator.LoadColorPalette(paletteFile)
	coordinator.Logger.Printf("Loaded color palette with %d colors", len(coordinator.Colors))

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
				coordinator.Logger.Printf("Generated image %d", task.ImageNumber)
				coordinator.ImageTasks[task.ImageNumber].Generated = true
			}
		}
	}
	close(coordinator.TasksDone)
	coordinator.Logger.Printf("Done generating images")

	coordinator.Logger.Printf("Shutting down")
}

func startWorker() {
	workerDone := make(chan bool, workerCount)

	for i := 0; i < workerCount; i++ {
		worker := newWorker(getLocalAddress(), 10001+i, workerDone)

		go worker.ProcessTasks()
	}

	for {
		if workerCount <= 0 {
			break
		}
		<-workerDone
		workerCount--
	}
}
