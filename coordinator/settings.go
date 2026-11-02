package coordinator

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BrugadaSyndrome/bslogger"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type settings struct {
	logger bslogger.Logger

	GenerateMovie      bool
	MandelbrotSettings mandelbrot.Settings
	RunName            string
	SavePath           string
	ServerAddress      string
	TaskGeneration     task.Generation
	TransitionSettings []transitionSettings
}

// settingsBackupName is the name of the copy of the settings saved in each run's folder
const settingsBackupName = "settings.json"

func NewSettings(settingsFile string) settings {
	s, err := readSettings(settingsFile)
	misc.CheckError(err, s.logger, misc.Fatal)
	s.logger.Debug(s.String())
	return s
}

func readSettings(settingsFile string) (settings, error) {
	s := settings{
		logger:        bslogger.NewLogger("CoordinatorSettings", bslogger.Normal, nil),
		ServerAddress: "",
	}
	err, fileBytes := misc.ReadFile(settingsFile)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(fileBytes, &s); err != nil {
		return s, fmt.Errorf("unable to parse %s: %w", settingsFile, err)
	}
	return s, s.Verify()
}

// loadRunSettings reads the settings saved in a run's folder, so a resumed run renders its remaining frames exactly like
// the ones already saved. The save path and run name come from the folder itself in case it was moved since.
func loadRunSettings(runFolder string) (settings, error) {
	runFolder, err := filepath.Abs(runFolder)
	if err != nil {
		return settings{}, err
	}

	settingsFile := filepath.Join(runFolder, settingsBackupName)
	if _, err := os.Stat(settingsFile); err != nil {
		// Older runs saved the settings under the name of the file they were loaded from
		candidates, _ := filepath.Glob(filepath.Join(runFolder, "*.json"))
		if len(candidates) != 1 {
			return settings{}, fmt.Errorf("expected %s or exactly one other .json file with the run's settings in %s, found %d .json files",
				settingsBackupName, runFolder, len(candidates))
		}
		settingsFile = candidates[0]
	}

	s, err := readSettings(settingsFile)
	if err != nil {
		return s, err
	}
	s.SavePath = filepath.Dir(runFolder)
	s.RunName = filepath.Base(runFolder)
	return s, nil
}

func defaultRunName(now time.Time) string {
	return "run_" + now.Format("2006_01_02-15_04_05")
}

// ffmpegAvailable reports whether ffmpeg can be run to make the movie
func ffmpegAvailable() bool {
	cmd := exec.Command("ffmpeg", "-version")
	output, err := cmd.Output()
	return err == nil && bytes.Contains(output, []byte(`ffmpeg version`))
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
		s.RunName = defaultRunName(time.Now())
	}
	if s.SavePath == "" {
		s.SavePath, _ = os.Getwd()
	}
	if s.ServerAddress == "" {
		s.ServerAddress = fmt.Sprintf("%s:%s", misc.GetLocalAddress(), "51000")
	}
	if s.TaskGeneration < task.Row || s.TaskGeneration > task.Grid {
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

	// GenerateMovie is left as configured even when ffmpeg is missing, since these settings are saved with the run and
	// a resumed run may have ffmpeg. The coordinator checks for ffmpeg itself when it starts.

	return nil
}
