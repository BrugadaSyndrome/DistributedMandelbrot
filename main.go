package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

/**
 * TODO
 * General
 * todo: get distributed mandelbrot working inside of a pi cluster
 * todo: handle tasks that do not get returned
 * todo: ways to find interesting zoom points
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
	var fileBytes []byte
	var err error

	// Read in settings
	if settingsFile != "" {
		err, fileBytes = readFile(settingsFile)
		if err != nil {
			log.Fatal(err.Error())
		}
		err = json.Unmarshal(fileBytes, &settings)
		if err != nil {
			log.Fatalf("ERROR - Unable to unmarshal %s - %s", settingsFile, err)
		}
	}

	// Verify (and fix) any settings that have been passed in
	err = settings.Verify()
	if err != nil {
		log.Fatalf("ERROR - Unable to use settings - %s", err)
	}
	log.Print(settings.String())

	coordinator := newCoordinator(settings, getLocalAddress(), 10000)
	coordinator.Logger.Print("Starting coordinator")

	// Create directory to store files from this run
	if _, err = os.Stat(filepath.Join(coordinator.Settings.SavePath, coordinator.Settings.RunName)); os.IsNotExist(err) {
		err = os.Mkdir(filepath.Join(coordinator.Settings.SavePath, coordinator.Settings.RunName), os.ModePerm)
		if err != nil {
			coordinator.Logger.Fatalf("ERROR - unable to create folder: %s", err)
		}
	}

	// Save settings to the directory as a backup
	bytesWritten, err := writeFile(filepath.Join(coordinator.Settings.SavePath, coordinator.Settings.RunName, settingsFile), fileBytes)
	if err != nil || bytesWritten == 0 {
		coordinator.Logger.Printf("INFO - was not able to make a backup copy of settingsFile: %s", settingsFile)
	}

	go coordinator.GenerateTasks()
	coordinator.Logger.Print("Waiting for workers to connect")

	coordinator.IngestTasks()

	coordinator.Logger.Print("Waiting for workers to shut down")
	coordinator.Wait.Wait()

	// All tasks returned from workers
	close(coordinator.TasksDone)

	// Generate movie
	if coordinator.Settings.GenerateMovie {
		digitCount := (int)(math.Log10((float64)(coordinator.ImageCount)) + 1)
		coordinator.Logger.Print("Making movie")
		args := []string{"-framerate", "1", "-r", "30", "-i", filepath.Join(coordinator.Settings.SavePath, coordinator.Settings.RunName, fmt.Sprintf("%%%dd.jpg", digitCount)), "-c:v", "libx264", "-pix_fmt", "yuvj420p", filepath.Join(coordinator.Settings.SavePath, coordinator.Settings.RunName, "movie.mp4")}
		cmd := exec.Command("ffmpeg", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			coordinator.Logger.Fatalf("ERROR - Unable to make movie: %v\n%s\n", err, stderr.String())
		}
		coordinator.Logger.Print("Done making movie\n")
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
