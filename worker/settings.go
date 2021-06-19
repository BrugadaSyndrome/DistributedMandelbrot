package worker

import (
	"encoding/json"
	"fmt"
	glog "log"
	"mandelbrot/log"
	"mandelbrot/misc"
)

type settings struct {
	logger log.Logger

	CoordinatorAddress string
}

func NewSettings(settingsFile string) settings {
	s := settings{
		logger: log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "WorkerSettings", log.Normal, nil),
	}
	err, bytes := misc.ReadFile(settingsFile)
	misc.CheckError(err, s.logger, misc.Fatal)
	misc.CheckError(json.Unmarshal(bytes, &s), s.logger, misc.Fatal)
	misc.CheckError(s.Verify(), s.logger, misc.Fatal)
	s.logger.Debug(s.String())
	return s
}

func (s *settings) String() string {
	output := "\nWorker settings\n"
	output += fmt.Sprintf("Coordinator Address: %s\n", s.CoordinatorAddress)
	return output
}

func (s *settings) Verify() error {
	if s.CoordinatorAddress == "" {
		s.CoordinatorAddress = fmt.Sprintf("%s:%s", misc.GetLocalAddress(), "51000")
	}
	return nil
}
