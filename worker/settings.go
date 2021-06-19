package worker

import (
	"encoding/json"
	"fmt"
	glog "log"
	"mandelbrot/log"
	"mandelbrot/misc"
)

type settings struct {
	CoordinatorAddress string
	Logger             log.Logger
	WorkerCount        int
}

func NewSettings(settingsFile string) settings {
	s := settings{
		Logger: log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "WorkerSettings", log.All, nil),
	}
	err, bytes := misc.ReadFile(settingsFile)
	misc.CheckError(err, s.Logger, misc.Fatal)
	misc.CheckError(json.Unmarshal(bytes, &s), s.Logger, misc.Fatal)
	misc.CheckError(s.Verify(), s.Logger, misc.Fatal)
	s.Logger.Debug(s.String())
	return s
}

func (s *settings) String() string {
	output := "\nWorker settings\n"
	output += fmt.Sprintf("Coordinator Address: %s\n", s.CoordinatorAddress)
	output += fmt.Sprintf("Worker Count: %d", s.WorkerCount)
	return output
}

func (s *settings) Verify() error {
	if s.CoordinatorAddress == "" {
		s.CoordinatorAddress = fmt.Sprintf("%s:%s", misc.GetLocalAddress(), "51000")
	}
	if s.WorkerCount <= 0 {
		s.WorkerCount = 3
	}
	return nil
}
