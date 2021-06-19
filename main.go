package main

import (
	"flag"
	glog "log"
	"mandelbrot/coordinator"
	"mandelbrot/log"
	"mandelbrot/worker"
)

var (
	logger       log.Logger
	mode         string
	settingsFile string
	workerCount  uint
)

func main() {
	flag.StringVar(&mode, "mode", "", "Specify if this instance is a 'coordinator' or 'worker'")
	flag.StringVar(&settingsFile, "settings", "", "Specify the file with the settings for this run")
	flag.UintVar(&workerCount, "workers", 2, "Specify the number of workers to create to process coordinator tasks")
	flag.Parse()

	logger = log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "Main", log.Normal, nil)

	switch mode {
	case "coordinator":
		startCoordinatorMode(settingsFile)
		break
	case "worker":
		startWorkerMode(settingsFile)
		break
	default:
		logger.Fatalf("Unknown mode '%s'. Please set the mode to 'coordinator' or 'worker'", mode)
	}
}

func startCoordinatorMode(settingsFile string) {
	logger.Info("Started Coordinator Mode")

	c := coordinator.NewCoordinator(settingsFile)

	c.Server.WG.Wait()
}

func startWorkerMode(settingsFile string) {
	logger.Info("Started Worker Mode")

	workers := make([]*worker.Worker, 0)
	var i uint
	for i = 0; i < workerCount; i++ {
		w := worker.NewWorker(settingsFile)
		workers = append(workers, &w)
	}

	for i := 0; i < 5; i++ {
		workers[i].ServerClient.Server.WG.Wait()
	}
}
