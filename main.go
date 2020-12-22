package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/jpeg"
	"log"
	"math"
	"os"
	"os/exec"
	"sync"
)

/**
 * TODO
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
		log.Fatalf("ERROR - Unknown mode '%s'. Please set the mode to 'coordinator' or 'worker'", mode)
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
			log.Fatalf("ERROR - Unable to unmarshal %s - %s", settingsFile, err)
		}
	}
	err := settings.Verify()
	if err != nil {
		log.Fatalf("ERROR - Unable to use settings - %s", err)
	}
	log.Print(settings.String())

	coordinator := newCoordinator(settings, getLocalAddress(), 10000)
	coordinator.Logger.Print("Starting coordinator")

	// Create directory to store files from this run
	if _, err = os.Stat(coordinator.Settings.RunName); os.IsNotExist(err) {
		err = os.Mkdir(coordinator.Settings.RunName, os.ModePerm)
		if err != nil {
			coordinator.Logger.Fatalf("ERROR - unable to create folder: %s", err)
		}
	}

	// todo: if web interface is being used then dont start generating tasks yet
	/*
		if settings.EnableWebInterface {
			_ = coordinator.StartWebInterface()
		}
	*/

	go coordinator.GenerateTasks()
	coordinator.Logger.Print("Waiting for workers to connect")

	digitCount := (int)(math.Log10((float64)(coordinator.ImageCount)) + 1)
	for c := 1; c <= coordinator.TaskCount; c++ {
		task := <-coordinator.TasksDone

		for it := 0; it < len(task.Colors); it++ {
			// Get the task
			imageTask, ok := coordinator.ImageTasks[task.ImageNumber]
			// Create the task to record the workers results
			if !ok {
				imageTask = &ImageTask{
					Image:      image.NewRGBA(coordinator.Rectangle),
					PixelsLeft: coordinator.PixelCount,
				}
			}

			// Draw the pixel on the image
			imageTask.Image.SetRGBA(it, task.Row, task.Colors[it])
			imageTask.PixelsLeft--

			// Save the task
			coordinator.ImageTasks[task.ImageNumber] = imageTask

			// Generate the image once all pixels are filled
			if imageTask.PixelsLeft == 0 {
				path := fmt.Sprintf("%[1]s/%0[2]*[3]d.jpg", coordinator.Settings.RunName, digitCount, task.ImageNumber)
				f, err := os.Create(path)
				if err != nil {
					coordinator.Logger.Fatalf("ERROR - Unable to create image: %s", err)
				}
				err = jpeg.Encode(f, imageTask.Image, nil)
				if err != nil {
					coordinator.Logger.Fatalf("ERROR - Unable to save image: %s", err)
				} else {
					// Remove the image to conserve memory
					coordinator.Mutex.Lock()
					delete(coordinator.ImageTasks, task.ImageNumber)
					coordinator.Mutex.Unlock()
					coordinator.Logger.Printf("Saved image to ./%s [completed images: %d/%d] [completed tasks: %d/%d]", path, task.ImageNumber+1, coordinator.ImageCount, c, coordinator.TaskCount)
				}
			}
		}
	}
	coordinator.Logger.Print("Done generating images")

	// Wait for workers to shut down
	coordinator.Logger.Print("Waiting for workers to shut down")
	coordinator.Wait.Wait()

	// All tasks returned from workers
	close(coordinator.TasksDone)

	// Generate movie
	if coordinator.Settings.GenerateMovie {
		coordinator.Logger.Print("Making movie")
		path, _ := os.Getwd()
		args := []string{"-framerate", "1", "-r", "30", "-i", fmt.Sprintf("%s\\%s\\%%%dd.jpg", path, coordinator.Settings.RunName, digitCount), "-c:v", "libx264", "-pix_fmt", "yuvj420p", fmt.Sprintf("%s\\%s\\movie.mp4", path, coordinator.Settings.RunName)}
		cmd := exec.Command("ffmpeg", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			coordinator.Logger.Fatalf("ERROR - Unable to make movie: %v\n%s\n", err, stderr.String())
		}
		coordinator.Logger.Print("Done making movie:\n")
	}

	// Nothing like a job well done
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
