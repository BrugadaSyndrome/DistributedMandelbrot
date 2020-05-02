package main

import (
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"sync"
)

/**
 * TODO
 *
 * Color
 * todo: When using smooth coloring, use hsl/hsv to make better color gradients
 * todo: Handle what to do when no palette is specified (default to a palette with just white in it and disable smooth coloring)
 * Cache iteration results in db
 * todo: [WIP] get distributed mandelbrot working inside of a multi-machine vagrant instance
 *     : including firewall stuff (avoid private network options because that wont be available normally)
 * todo: stashing results in mysql db
 * Zoom
 * todo: improve zoom by allowing 'sliding' zooms from (x0, y0) => (x1, y1)
 */

var (
	boundary, centerX, centerY, magnificationEnd, magnificationStart, magnificationStep float64
	height, maxIterations, width, workerCount                                           int
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

	coordinator.LoadColorPalette(paletteFile)
	coordinator.Logger.Printf("Loaded color palette with %d colors", len(coordinator.Colors))

	go coordinator.GenerateTasks()

	for c := 1; c <= coordinator.TaskCount; c++ {
		task := <-coordinator.TasksDone

		for it := 0; it < len(task.Iterations); it++ {

			finalColor := coordinator.GetColor(task.Iterations[it])
			if int(math.Floor(task.Iterations[it])) != coordinator.Settings.MaxIterations && coordinator.Settings.SmoothColoring {
				color1 := coordinator.GetColor(task.Iterations[it])
				color2 := coordinator.GetColor(task.Iterations[it] + 1)

				_, fraction := math.Modf(task.Iterations[it])
				finalColor = color.RGBA{
					uint8(float64(color2.R-color1.R)*fraction) + color1.R,
					uint8(float64(color2.G-color1.G)*fraction) + color1.G,
					uint8(float64(color2.B-color1.B)*fraction) + color1.B,
					255,
				}
			}

			coordinator.ImageTasks[task.ImageNumber].Image.SetRGBA(it, task.Row, finalColor)
			coordinator.ImageTasks[task.ImageNumber].PixelsLeft--
			if coordinator.ImageTasks[task.ImageNumber].PixelsLeft == 0 {
				name := fmt.Sprintf("images/%d.png", task.ImageNumber)
				f, _ := os.Create(name)
				png.Encode(f, coordinator.ImageTasks[task.ImageNumber].Image)
				coordinator.ImageTasks[task.ImageNumber].Generated = true
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
