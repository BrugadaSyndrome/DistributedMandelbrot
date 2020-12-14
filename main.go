package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image/png"
	"log"
	"os"
	"sync"
)

/**
 * TODO
 * Settings
 * todo: add option to specify if a png or jpeg will be generated
 * todo: add option to generate movie (as well as a way to generate a movie)
 * Documentation
 * todo: update readme to reflect major changes
 * Web Interface
 * todo: figure out how it should work (settings, coordinator and worker tabs maybe)
 * Zoom
 * todo: improve zoom by allowing 'sliding' zooms from (x0, y0) => (x1, y1)
 * Cache iteration results in db
 * todo: get distributed mandelbrot working inside of a multi-machine vagrant instance
 *     : including firewall stuff (avoid private network options because that wont be available normally)
 * todo: stashing results in mysql db
 * Color
 * todo: Add in other color interpolation options (HSV, HSL, LAB, ...)
 * todo: Look into allowing the use of the exterior distance estimation technique
 */

var (
	mode, settingsFile string
	templates          *template.Template
)

func main() {
	parseArguments()

	switch mode {
	case "coordinator":
		startCoordinatorMode(settingsFile)
		break
	case "worker":
		startWorkerMode(settingsFile)
		break
	default:
		log.Fatalf("Unknown mode '%s'. Please set the mode to 'coordinator' or 'worker'", mode)
	}
}

func startCoordinatorMode(settingsFile string) {
	var settings CoordinatorSettings

	// Read in settings
	if settingsFile != "" {
		err, fileBytes := readFile(settingsFile)
		if err != nil {
			log.Fatal(err.Error())
		}
		err = json.Unmarshal(fileBytes, &settings)
		if err != nil {
			log.Fatalf("Unable to unmarshal %s - %s", settingsFile, err)
		}
	}
	err := settings.Verify()
	if err != nil {
		log.Fatalf("Unable to use settings - %s", err)
	}
	log.Print(settings.String())

	coordinator := newCoordinator(settings, getLocalAddress(), 10000)
	coordinator.Logger.Print("Starting coordinator")

	// todo: if web interface is being used then dont start generating tasks yet
	if settings.EnableWebInterface {
		_ = coordinator.StartWebInterface()
	}

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
				coordinator.ImageTasks[task.ImageNumber].Completed = true
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

func startWorkerMode(settingsFile string) {
	var wg sync.WaitGroup
	var settings WorkerSettings

	// Read in settings
	if settingsFile != "" {
		err, fileBytes := readFile(settingsFile)
		if err != nil {
			log.Fatal(err.Error())
		}
		err = json.Unmarshal(fileBytes, &settings)
		if err != nil {
			log.Fatalf("Unable to unmarshal %s - %s", settingsFile, err)
		}
	}
	err := settings.Verify()
	if err != nil {
		log.Fatalf("Unable to use settings - %s", err)
	}
	log.Print(settings.String())

	// Start up requested amount of workers
	for i := 0; i < settings.WorkerCount; i++ {
		worker := newWorker(settings, i, &wg)
		wg.Add(1)
		go worker.ProcessTasks()
	}

	// Wait for all workers to be done with their work
	wg.Wait()
}
