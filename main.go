package main

import (
	"fmt"
	"image/png"
	"os"
	"sync"
)

/**
 * TODO
 *
 * Zoom
 * todo: improve zoom by allowing 'sliding' zooms from (x0, y0) => (x1, y1)
 * Color
 * ? todo: modify color classes to implement the colors.Color interface
 * ? todo: Use the new RGB/HSV classes to for db stuff and for coloring the image (also flesh out the palette table so we can specify a palette id in the cli parameters)
 * todo: When using smooth coloring, use hsl/hsv to make better color gradients
 * todo: Look into allowing the use of the exterior distance estimation technique
 * Cache iteration results in db
 * todo: [WIP] get distributed mandelbrot working inside of a multi-machine vagrant instance
 *     : including firewall stuff (avoid private network options because that wont be available normally)
 * todo: [WIP] stashing results in mysql db
 */

var (
	boundary, centerX, centerY, magnificationEnd, magnificationStart, magnificationStep float64
	height, maxIterations, superSampling, width, workerCount                            int
	coordinatorAddress, paletteFile                                                     string
	isWorker, isCoordinator, smoothColoring                                             bool
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

func startCoordinator() {
	coordinator := newCoordinator(getLocalAddress(), 10000)
	coordinator.Logger.Print("Starting coordinator")

	if paletteFile != "" {
		coordinator.LoadColorPalette(paletteFile)
	}
	coordinator.Logger.Printf("Loaded color palette with %d colors", len(coordinator.Settings.Colors))

	go coordinator.GenerateTasks()

	for c := 1; c <= coordinator.TaskCount; c++ {
		task := <-coordinator.TasksDone

		for it := 0; it < len(task.Colors); it++ {
			// Draw pixel on the image
			coordinator.ImageTasks[task.ImageNumber].Image.SetRGBA(it, task.Row, task.Colors[it])
			coordinator.ImageTasks[task.ImageNumber].PixelsLeft--

			// Generate the image once all pixels are filled
			if coordinator.ImageTasks[task.ImageNumber].PixelsLeft == 0 {
				name := fmt.Sprintf("images/%d.png", task.ImageNumber)
				f, _ := os.Create(name)
				png.Encode(f, coordinator.ImageTasks[task.ImageNumber].Image)
				coordinator.Mutex.Lock()
				coordinator.ImageTasks[task.ImageNumber].Generated = true
				coordinator.Mutex.Unlock()
				coordinator.Logger.Printf("Generated image %d [completed tasks %d/%d]", task.ImageNumber, c, coordinator.TaskCount)
			}
		}
	}
	coordinator.Logger.Print("Done generating images")

	// Wait for workers to shut down
	coordinator.Logger.Print("Waiting for workers to shut down")
	coordinator.Wait.Wait()

	// All tasks returned from workers
	close(coordinator.TasksDone)
	coordinator.Logger.Print("Shutting down")
}

func startWorker() {
	var wg sync.WaitGroup

	// Start up request amount of workers
	for i := 0; i < workerCount; i++ {
		worker := newWorker(getLocalAddress(), 10001+i, &wg)
		wg.Add(1)
		go worker.ProcessTasks()
	}

	// Wait for all workers to be done with their work
	wg.Wait()
}
