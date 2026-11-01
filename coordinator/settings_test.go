package coordinator

import (
	"DistributedMandelbrot/task"
	"os"
	"strings"
	"testing"

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
