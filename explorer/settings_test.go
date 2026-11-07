package explorer

import (
	"DistributedMandelbrot/mandelbrot"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLimitsVerify(t *testing.T) {
	tests := []struct {
		name string
		in   limits
		want limits
	}{
		{"defaults", limits{}, limits{MaxIterations: 10000, MaxSuperSampling: 4, MaxZoomLevel: 39, MinZoomLevel: 0}},
		{"kept when valid", limits{MaxIterations: 500, MaxSuperSampling: 2, MaxZoomLevel: 20, MinZoomLevel: 3},
			limits{MaxIterations: 500, MaxSuperSampling: 2, MaxZoomLevel: 20, MinZoomLevel: 3}},
		{"max zoom capped to float64 precision", limits{MaxSuperSampling: 4, MaxZoomLevel: 60},
			limits{MaxIterations: 10000, MaxSuperSampling: 4, MaxZoomLevel: 39}},
		{"less super sampling allows deeper zooms", limits{MaxSuperSampling: 1, MaxZoomLevel: 60},
			limits{MaxIterations: 10000, MaxSuperSampling: 1, MaxZoomLevel: 41}},
		{"min zoom not below the lowest level", limits{MinZoomLevel: -50},
			limits{MaxIterations: 10000, MaxSuperSampling: 4, MaxZoomLevel: 39, MinZoomLevel: lowestZoomLevel}},
		{"min zoom not above max zoom", limits{MaxZoomLevel: 10, MinZoomLevel: 12},
			limits{MaxIterations: 10000, MaxSuperSampling: 4, MaxZoomLevel: 10, MinZoomLevel: 10}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in
			got.verify()
			if got != tc.want {
				t.Errorf("verify() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLimitsClamp(t *testing.T) {
	l := limits{MaxIterations: 10000, MaxSuperSampling: 4}
	s := mandelbrot.Settings{MaxIterations: 100000000, SuperSampling: 8}
	l.clamp(&s)
	if s.MaxIterations != 10000 || s.SuperSampling != 4 {
		t.Errorf("clamp gave MaxIterations %d and SuperSampling %d, want 10000 and 4", s.MaxIterations, s.SuperSampling)
	}
	s = mandelbrot.Settings{MaxIterations: 750, SuperSampling: 2}
	l.clamp(&s)
	if s.MaxIterations != 750 || s.SuperSampling != 2 {
		t.Errorf("clamp changed values inside the limits to %d and %d", s.MaxIterations, s.SuperSampling)
	}
}

func TestLimitsCheckRender(t *testing.T) {
	l := limits{MaxIterations: 10000, MaxSuperSampling: 4, MaxZoomLevel: 20, MinZoomLevel: 0}
	valid := renderRequest{Boundary: 1000, MaxIterations: 1000, SuperSampling: 2, Tiles: []tileKey{{2, -2, -1}, {20, 0, 0}}}
	if err := l.checkRender(valid); err != nil {
		t.Fatalf("checkRender rejected a valid request: %s", err)
	}

	tooMany := make([]tileKey, maxTilesPerRender+1)
	tests := []struct {
		name   string
		change func(r *renderRequest)
		want   string
	}{
		{"too many iterations", func(r *renderRequest) { r.MaxIterations = 100000000 }, "MaxIterations"},
		{"no iterations", func(r *renderRequest) { r.MaxIterations = 0 }, "MaxIterations"},
		{"too much super sampling", func(r *renderRequest) { r.SuperSampling = 5 }, "SuperSampling"},
		{"no super sampling", func(r *renderRequest) { r.SuperSampling = 0 }, "SuperSampling"},
		{"zero boundary", func(r *renderRequest) { r.Boundary = 0 }, "Boundary"},
		{"NaN boundary", func(r *renderRequest) { r.Boundary = math.NaN() }, "Boundary"},
		{"infinite boundary", func(r *renderRequest) { r.Boundary = math.Inf(1) }, "Boundary"},
		{"too many tiles", func(r *renderRequest) { r.Tiles = tooMany }, "at most"},
		{"too deep", func(r *renderRequest) { r.Tiles = []tileKey{{21, 0, 0}} }, "zoom levels"},
		{"too shallow", func(r *renderRequest) { r.Tiles = []tileKey{{-1, 0, 0}} }, "zoom levels"},
		{"outside the plane", func(r *renderRequest) { r.Tiles = []tileKey{{0, 5, 0}} }, "outside the plane"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := valid
			tc.change(&r)
			err := l.checkRender(r)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("checkRender error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestReadSettings(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings_explorer.json")
	contents := `{
		"HttpAddress": "127.0.0.1:9000",
		"ServerAddress": "127.0.0.1:51001",
		"MandelbrotSettings": {"MaxIterations": 50000, "SuperSampling": 6, "Boundary": 1000},
		"Limits": {"MaxIterations": 20000, "MaxZoomLevel": 30},
		"StartX": -0.5, "StartY": 9, "StartZoom": 45
	}`
	if err := os.WriteFile(file, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := readSettings(file)
	if err != nil {
		t.Fatalf("readSettings: %s", err)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"HttpAddress", s.HttpAddress, "127.0.0.1:9000"},
		{"CachePath default", s.CachePath, "explorer_cache"},
		{"CacheMaxMB default", s.CacheMaxMB, uint(2048)},
		{"MaxIterations limit", s.Limits.MaxIterations, uint(20000)},
		{"MaxSuperSampling default", s.Limits.MaxSuperSampling, 4},
		{"default MaxIterations clamped", s.MandelbrotSettings.MaxIterations, uint(20000)},
		{"default SuperSampling clamped", s.MandelbrotSettings.SuperSampling, 4},
		{"StartX kept", s.StartX, -0.5},
		{"StartY outside the plane reset", s.StartY, 0.0},
		{"StartZoom clamped to MaxZoomLevel", s.StartZoom, 30.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if len(s.MandelbrotSettings.Palette) == 0 {
		t.Error("Verify should have generated a palette")
	}
}

func TestReadSettingsErrors(t *testing.T) {
	if _, err := readSettings(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("readSettings of a missing file should fail")
	}
	file := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(file, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSettings(file); err == nil {
		t.Error("readSettings of invalid JSON should fail")
	}
}
