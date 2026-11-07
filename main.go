package main

import (
	"DistributedMandelbrot/coordinator"
	"DistributedMandelbrot/explorer"
	"DistributedMandelbrot/worker"
	"flag"
	"github.com/BrugadaSyndrome/bslogger"
	"runtime"
)

const (
	// defaultWorkerCount is how many workers a worker process starts when -workers is not given
	defaultWorkerCount = 2
	// defaultExplorerWorkerCount is how many workers the explorer starts in its own process when -workers is not given
	defaultExplorerWorkerCount = 4
)

var (
	logger       bslogger.Logger
	mode         string
	resumeFolder string
	settingsFile string
	workerCount  uint
)

func main() {
	flag.StringVar(&mode, "mode", "", "Specify if this instance is a 'coordinator', 'worker' or 'explorer'")
	flag.StringVar(&resumeFolder, "resume", "", "Coordinator only: the folder of an unfinished run to continue with the settings saved in it")
	flag.StringVar(&settingsFile, "settings", "", "Specify the file with the settings for this run")
	flag.UintVar(&workerCount, "workers", 0, "Specify the number of workers to create: 2 by default for a worker process, 4 for the explorer (at most one per core)")
	flag.Parse()

	logger = bslogger.NewLogger("Main", bslogger.Normal, nil)

	switch mode {
	case "coordinator":
		startCoordinatorMode(settingsFile, resumeFolder)
		break
	case "worker":
		startWorkerMode(settingsFile)
		break
	case "explorer":
		startExplorerMode(settingsFile)
		break
	default:
		logger.Fatalf("Unknown mode '%s'. Please set the mode to 'coordinator', 'worker' or 'explorer'", mode)
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

	count := workerCount
	if count == 0 {
		count = defaultWorkerCount
	}
	workers := make([]*worker.Worker, count)
	var i uint
	for i = 0; i < count; i++ {
		workers[i] = worker.NewWorker(settingsFile)
	}

	for i = 0; i < count; i++ {
		workers[i].ServerClient.Server.Wait()
	}
}

// startExplorerMode serves the explorer page with workers in this process. Worker processes on other machines can
// join too by pointing their CoordinatorAddress at the explorer's ServerAddress.
func startExplorerMode(settingsFile string) {
	logger.Info("Started Explorer Mode")

	e := explorer.NewExplorer(settingsFile)
	cores := uint(runtime.NumCPU())
	count, lowered := explorerWorkerCount(workerCount, cores)
	if lowered {
		logger.Warningf("Starting %d workers instead of %d since this machine has %d cores", count, workerCount, cores)
	}
	for range count {
		worker.NewWorkerFor(e.RPCAddress())
	}

	e.Wait()
}

// explorerWorkerCount is how many workers the explorer starts: the number requested, or defaultExplorerWorkerCount if
// none was, but never more than one per core. lowered reports that the request was more than the cores allow.
func explorerWorkerCount(requested uint, cores uint) (count uint, lowered bool) {
	cores = max(cores, 1)
	if requested == 0 {
		return min(defaultExplorerWorkerCount, cores), false
	}
	if requested > cores {
		return cores, true
	}
	return requested, false
}
