package main

import (
	"DistributedMandelbrot/coordinator"
	"DistributedMandelbrot/worker"
	"flag"
	"github.com/BrugadaSyndrome/bslogger"
)

var (
	logger       bslogger.Logger
	mode         string
	settingsFile string
	workerCount  uint
)

func main() {
	flag.StringVar(&mode, "mode", "", "Specify if this instance is a 'coordinator' or 'worker'")
	flag.StringVar(&settingsFile, "settings", "", "Specify the file with the settings for this run")
	flag.UintVar(&workerCount, "workers", 2, "Specify the number of workers to create to process coordinator tasks")
	flag.Parse()

	logger = bslogger.NewLogger("Main", bslogger.Normal, nil)

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

	c.Server.Wait()
}

func startWorkerMode(settingsFile string) {
	logger.Info("Started Worker Mode")

	workers := make([]*worker.Worker, workerCount)
	var i uint
	for i = 0; i < workerCount; i++ {
		w := worker.NewWorker(settingsFile)
		workers[i] = &w
	}

	for i = 0; i < workerCount; i++ {
		workers[i].ServerClient.Server.Wait()
	}
}
