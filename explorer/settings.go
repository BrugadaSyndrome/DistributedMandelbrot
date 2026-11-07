package explorer

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/BrugadaSyndrome/bslogger"
)

// limits are the server side restrictions on what the page may ask for. They are enforced on every request, whatever
// the form sends, so one request cannot tie up every worker.
type limits struct {
	MaxIterations    uint
	MaxSuperSampling int
	MaxZoomLevel     int
	MinZoomLevel     int
}

type settings struct {
	logger bslogger.Logger

	CacheMaxMB  uint
	CachePath   string
	HttpAddress string
	Limits      limits
	// MandelbrotSettings are the defaults shown in the page's form. Width, Height and the movie settings are not used.
	MandelbrotSettings mandelbrot.Settings
	ServerAddress      string
	StartX             float64
	StartY             float64
	StartZoom          float64 // zoom levels may be fractional on screen; tiles are rendered at whole levels
}

func NewSettings(settingsFile string) settings {
	s, err := readSettings(settingsFile)
	misc.CheckError(err, s.logger, misc.Fatal)
	return s
}

func readSettings(settingsFile string) (settings, error) {
	s := settings{logger: bslogger.NewLogger("ExplorerSettings", bslogger.Normal, nil)}
	err, fileBytes := misc.ReadFile(settingsFile)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(fileBytes, &s); err != nil {
		return s, fmt.Errorf("unable to parse %s: %w", settingsFile, err)
	}
	return s, s.Verify()
}

func (s *settings) Verify() error {
	if s.CacheMaxMB == 0 {
		s.CacheMaxMB = 2048
	}
	if s.CachePath == "" {
		s.CachePath = "explorer_cache"
	}
	if s.HttpAddress == "" {
		// Only reachable from this machine by default, since the page has no login
		s.HttpAddress = "127.0.0.1:8080"
	}
	if s.ServerAddress == "" {
		s.ServerAddress = fmt.Sprintf("%s:%s", misc.GetLocalAddress(), "51000")
	}
	s.Limits.verify()

	misc.CheckError(s.MandelbrotSettings.Verify(), s.logger, misc.Fatal)
	s.Limits.clamp(&s.MandelbrotSettings)

	if math.Abs(s.StartX) > planeLimit || math.IsNaN(s.StartX) {
		s.StartX = -0.75
	}
	if math.Abs(s.StartY) > planeLimit || math.IsNaN(s.StartY) {
		s.StartY = 0
	}
	if math.IsNaN(s.StartZoom) {
		s.StartZoom = 0
	}
	s.StartZoom = min(max(s.StartZoom, float64(s.Limits.MinZoomLevel)), float64(s.Limits.MaxZoomLevel))
	return nil
}

func (l *limits) verify() {
	if l.MaxIterations == 0 {
		l.MaxIterations = 10000
	}
	if l.MaxSuperSampling < 1 {
		l.MaxSuperSampling = 4
	}
	// Never deeper than float64 can render cleanly with the most super sampling allowed
	deepest := maxPreciseLevel(l.MaxSuperSampling)
	if l.MaxZoomLevel <= 0 || l.MaxZoomLevel > deepest {
		l.MaxZoomLevel = deepest
	}
	l.MinZoomLevel = min(max(l.MinZoomLevel, lowestZoomLevel), l.MaxZoomLevel)
}

// clamp brings settings from the page (or the defaults) within the limits
func (l *limits) clamp(s *mandelbrot.Settings) {
	s.MaxIterations = min(s.MaxIterations, l.MaxIterations)
	s.SuperSampling = min(s.SuperSampling, l.MaxSuperSampling)
}

// checkRender rejects a render request that is outside the limits. The page normalizes its settings through
// /api/settings first, so only a hand made request gets here with bad values.
func (l *limits) checkRender(r renderRequest) error {
	switch {
	case r.MaxIterations < 1 || r.MaxIterations > l.MaxIterations:
		return fmt.Errorf("MaxIterations must be between 1 and %d", l.MaxIterations)
	case r.SuperSampling < 1 || r.SuperSampling > l.MaxSuperSampling:
		return fmt.Errorf("SuperSampling must be between 1 and %d", l.MaxSuperSampling)
	case !(r.Boundary > 0) || math.IsInf(r.Boundary, 0):
		return errors.New("Boundary must be a positive number")
	case len(r.Tiles) > maxTilesPerRender:
		return fmt.Errorf("at most %d tiles can be rendered at once", maxTilesPerRender)
	}
	for _, k := range r.Tiles {
		if k.Level < l.MinZoomLevel || k.Level > l.MaxZoomLevel {
			return fmt.Errorf("tile %s is outside zoom levels %d to %d", k, l.MinZoomLevel, l.MaxZoomLevel)
		}
		if !k.inPlane() {
			return fmt.Errorf("tile %s is outside the plane", k)
		}
	}
	return nil
}
