package coordinator

import (
	"DistributedMandelbrot/task"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
)

// newTestSettings returns settings with a fixed server address so Verify does not need to look up a network interface
func newTestSettings() settings {
	return settings{
		logger:        bslogger.NewLogger("CoordinatorSettingsTest", bslogger.Normal, nil),
		ServerAddress: "127.0.0.1:51000",
	}
}

func TestSettingsVerifyDefaults(t *testing.T) {
	s := newTestSettings()
	if err := s.Verify(); err != nil {
		t.Fatalf("Verify() returned error: %s", err)
	}

	if !strings.HasPrefix(s.RunName, "run_") {
		t.Errorf("RunName = %q, want a run_<timestamp> default", s.RunName)
	}
	cwd, _ := os.Getwd()
	if s.SavePath != cwd {
		t.Errorf("SavePath = %q, want the working directory %q", s.SavePath, cwd)
	}
	if s.TaskGeneration != task.Row {
		t.Errorf("TaskGeneration = %s, want Row", s.TaskGeneration)
	}
	if len(s.TransitionSettings) != 1 {
		t.Fatalf("len(TransitionSettings) = %d, want 1 default transition", len(s.TransitionSettings))
	}
	if s.MandelbrotSettings.Width == 0 || s.MandelbrotSettings.Height == 0 {
		t.Error("MandelbrotSettings should be verified and given default dimensions")
	}
}

func TestSettingsVerifyTaskGeneration(t *testing.T) {
	tests := []struct {
		in, want task.Generation
	}{
		{task.Row, task.Row},
		{task.Column, task.Column},
		{task.Image, task.Image},
		{task.Grid, task.Grid},
		{-1, task.Row},
		{task.Grid + 1, task.Row},
	}
	for _, tc := range tests {
		s := newTestSettings()
		s.TaskGeneration = tc.in
		_ = s.Verify()
		if s.TaskGeneration != tc.want {
			t.Errorf("TaskGeneration %d verified to %s, want %s", int(tc.in), s.TaskGeneration, tc.want)
		}
	}
}

func TestSettingsVerifyKeepsRunNameAndSavePath(t *testing.T) {
	s := newTestSettings()
	s.RunName = "my_run"
	s.SavePath = t.TempDir()
	want := s.SavePath
	_ = s.Verify()
	if s.RunName != "my_run" || s.SavePath != want {
		t.Errorf("Verify() changed RunName/SavePath to %q/%q", s.RunName, s.SavePath)
	}
}

func TestTransitionSettingsVerify(t *testing.T) {
	t.Run("out of range values are replaced with defaults", func(t *testing.T) {
		ts := transitionSettings{StartX: 5, StartY: -5, EndX: 4.5, EndY: -10, MagnificationStep: 1}
		_ = ts.Verify()
		want := transitionSettings{MagnificationStart: 0.5, MagnificationEnd: 1.5, MagnificationStep: 1.1}
		if ts != want {
			t.Errorf("got %+v, want %+v", ts, want)
		}
	})

	t.Run("valid values are kept", func(t *testing.T) {
		ts := transitionSettings{StartX: -0.5, EndX: -0.76, EndY: -0.08, MagnificationStart: 1, MagnificationEnd: 250, MagnificationStep: 1.25}
		want := ts
		_ = ts.Verify()
		if ts != want {
			t.Errorf("got %+v, want %+v", ts, want)
		}
	})
}

// A machine without ffmpeg must not overwrite the choice in the settings, since that choice is saved with the run and
// a resumed run on another machine should still make the movie
func TestSettingsVerifyKeepsGenerateMovie(t *testing.T) {
	s := newTestSettings()
	s.GenerateMovie = true
	_ = s.Verify()
	if !s.GenerateMovie {
		t.Error("Verify() turned GenerateMovie off")
	}
}

func TestDefaultRunNameUses24HourClock(t *testing.T) {
	afternoon := time.Date(2026, 11, 1, 15, 21, 13, 0, time.Local)
	if got, want := defaultRunName(afternoon), "run_2026_11_01-15_21_13"; got != want {
		t.Errorf("defaultRunName = %q, want %q", got, want)
	}
}

// writeRunSettings saves settings JSON with a recognizable MaxIterations into a run folder under the given name
func writeRunSettings(t *testing.T, folder string, name string, maxIterations uint) {
	t.Helper()
	s := newTestSettings()
	s.MandelbrotSettings.MaxIterations = maxIterations
	s.RunName = "name_from_the_original_run"
	s.SavePath = "path/from/the/original/run"
	contents, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, name), contents, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRunSettings(t *testing.T) {
	tests := []struct {
		name              string
		files             map[string]uint // file name to the MaxIterations saved in it
		wantMaxIterations uint            // 0 means loading should fail
	}{
		{"settings.json", map[string]uint{settingsBackupName: 123}, 123},
		{"older runs saved the settings under their original name", map[string]uint{"settings_coordinator.json": 456}, 456},
		{"settings.json wins over other json files", map[string]uint{settingsBackupName: 123, "notes.json": 789}, 123},
		{"no settings", map[string]uint{}, 0},
		{"ambiguous older run", map[string]uint{"a.json": 1, "b.json": 2}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			folder := filepath.Join(t.TempDir(), "my_run")
			if err := os.Mkdir(folder, os.ModePerm); err != nil {
				t.Fatal(err)
			}
			for name, maxIterations := range tc.files {
				writeRunSettings(t, folder, name, maxIterations)
			}

			s, err := loadRunSettings(folder)
			if tc.wantMaxIterations == 0 {
				if err == nil {
					t.Fatal("loadRunSettings should fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("loadRunSettings returned error: %s", err)
			}
			if s.MandelbrotSettings.MaxIterations != tc.wantMaxIterations {
				t.Errorf("MaxIterations = %d, want %d", s.MandelbrotSettings.MaxIterations, tc.wantMaxIterations)
			}
			// The folder may have been moved since the run started, so its location comes from the path given
			if s.RunName != "my_run" || s.SavePath != filepath.Dir(folder) {
				t.Errorf("SavePath/RunName = %q/%q, want %q/%q", s.SavePath, s.RunName, filepath.Dir(folder), "my_run")
			}
		})
	}
}
