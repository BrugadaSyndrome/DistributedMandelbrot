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
	resumeFolder string
	settingsFile string
	workerCount  uint
)

func main() {
	flag.StringVar(&mode, "mode", "", "Specify if this instance is a 'coordinator' or 'worker'")
	flag.StringVar(&resumeFolder, "resume", "", "Coordinator only: the folder of an unfinished run to continue with the settings saved in it")
	flag.StringVar(&settingsFile, "settings", "", "Specify the file with the settings for this run")
	flag.UintVar(&workerCount, "workers", 2, "Specify the number of workers to create to process coordinator tasks")
	flag.Parse()

	logger = bslogger.NewLogger("Main", bslogger.Normal, nil)

	switch mode {
	case "coordinator":
		startCoordinatorMode(settingsFile, resumeFolder)
		break
	case "worker":
		startWorkerMode(settingsFile)
		break
	default:
		logger.Fatalf("Unknown mode '%s'. Please set the mode to 'coordinator' or 'worker'", mode)
	}
}

func startCoordinatorMode(settingsFile string, resumeFolder string) {
	logger.Info("Started Coordinator Mode")

	var c *coordinator.Coordinator
	if resumeFolder != "" {
		if settingsFile != "" {
			logger.Warningf("Ignoring -settings=%s since a resumed run uses the settings saved in %s", settingsFile, resumeFolder)
		}
		c = coordinator.ResumeCoordinator(resumeFolder)
	} else {
		c = coordinator.NewCoordinator(settingsFile)
	}

	c.Server.Wait()
}

func startWorkerMode(settingsFile string) {
	logger.Info("Started Worker Mode")

	workers := make([]*worker.Worker, workerCount)
	var i uint
	for i = 0; i < workerCount; i++ {
		workers[i] = worker.NewWorker(settingsFile)
	}

	for i = 0; i < workerCount; i++ {
		workers[i].ServerClient.Server.Wait()
	}
}
