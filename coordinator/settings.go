package coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	glog "log"
	"mandelbrot/log"
	"mandelbrot/mandelbrot"
	"mandelbrot/misc"
	"mandelbrot/task"
	"os"
	"os/exec"
	"time"
)

type settings struct {
	logger log.Logger

	GenerateMovie      bool
	MandelbrotSettings mandelbrot.Settings
	RunName            string
	SavePath           string
	ServerAddress      string
	TaskGeneration     task.Generation
	TransitionSettings []transitionSettings
}

func NewSettings(settingsFile string) settings {
	s := settings{
		logger:        log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "CoordinatorSettings", log.Normal, nil),
		ServerAddress: "",
	}
	err, fileBytes := misc.ReadFile(settingsFile)
	misc.CheckError(err, s.logger, misc.Fatal)
	misc.CheckError(json.Unmarshal(fileBytes, &s), s.logger, misc.Fatal)
	misc.CheckError(s.Verify(), s.logger, misc.Fatal)
	s.logger.Debug(s.String())
	return s
}

func (s *settings) String() string {
	output := "\nCoordinator settings\n"
	output += fmt.Sprintf("My Address: %s", s.ServerAddress)
	return output
}

func (s *settings) Verify() error {
	// GenerateMovie defaults to false already
	misc.CheckError(s.MandelbrotSettings.Verify(), s.logger, misc.Fatal)
	if s.RunName == "" {
		s.RunName = "run_" + time.Now().Format("2006_01_02-03_04_05")
	}
	if s.SavePath == "" {
		s.SavePath, _ = os.Getwd()
	}
	if s.ServerAddress == "" {
		s.ServerAddress = fmt.Sprintf("%s:%s", misc.GetLocalAddress(), "51000")
	}
	if s.TaskGeneration < task.Row || s.TaskGeneration > task.Image {
		s.TaskGeneration = task.Row
	}
	if len(s.TransitionSettings) == 0 {
		s.TransitionSettings = []transitionSettings{
			{
				FrameCount:         1,
				MagnificationStart: 0.5,
				MagnificationEnd:   1.5,
				MagnificationStep:  1.1,
			},
		}
	}

	// Verify each of the transition settings objects
	for i := 0; i < len(s.TransitionSettings); i++ {
		misc.CheckError(s.TransitionSettings[i].Verify(), s.logger, misc.Warning)
	}

	// If generate movie is set to true, verify ffmpeg is setup
	if s.GenerateMovie {
		cmd := exec.Command("ffmpeg")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		misc.CheckError(cmd.Run(), s.logger, misc.Warning)
		if !bytes.Contains(stderr.Bytes(), []byte(`ffmpeg version`)) {
			s.GenerateMovie = false
			s.logger.Info("Ffmpeg is not installed. Disabling GenerateMovie.")
		}
	}

	return nil
}
