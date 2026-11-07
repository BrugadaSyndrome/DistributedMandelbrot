package explorer

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
)

//go:embed web
var webFiles embed.FS

// maxTilesPerRender caps one render request. A 4K screen shows about 200 tiles at a whole zoom level and up to four
// times as many between levels.
const maxTilesPerRender = 1024

// maxRequestBytes caps the JSON body of any request
const maxRequestBytes = 1 << 20

// frameHeaderSize is the size of the header before each tile in a render stream, all little endian:
// level int32 | flags uint32 | x int64 | y int64 | length uint32
const frameHeaderSize = 28

// frameCached is set in a frame's flags when the tile came from the cache
const frameCached = 1

type renderRequest struct {
	Session       string
	Boundary      float64
	MaxIterations uint
	SuperSampling int
	Tiles         []tileKey // most wanted first
}

type sessionRequest struct {
	Session string
}

type settingsRequest struct {
	Session  string
	Settings mandelbrot.Settings
}

// configResponse is what the page needs to start
type configResponse struct {
	Defaults      mandelbrot.Settings
	LevelZeroSpan float64
	Limits        limits
	PlaneLimit    float64
	StartX        float64
	StartY        float64
	StartZoom     float64
	TileSize      int
}

// statusResponse answers a heartbeat so the page can show what the explorer is doing
type statusResponse struct {
	CacheBytes int64
	InProgress int
	Queued     int
	Workers    int
}

type errorResponse struct {
	Error string
}

// Explorer serves the explorer page and streams tiles to it, computed by workers through its Coordinator or read from
// the cache.
type Explorer struct {
	cache       *tileCache
	coordinator *Coordinator
	logger      bslogger.Logger
	sessions    *sessionGuard
	settings    settings

	renderMutex  sync.Mutex
	cancelRender context.CancelFunc // stops the current render stream when a newer one starts

	httpDone chan error
}

// NewExplorer starts the RPC server for workers and the web server for the page
func NewExplorer(settingsFile string) *Explorer {
	s := NewSettings(settingsFile)
	logger := bslogger.NewLogger("Explorer", bslogger.Normal, nil)
	cache, err := openTileCache(s.CachePath, int64(s.CacheMaxMB)<<20)
	misc.CheckError(err, logger, misc.Fatal)
	e := newExplorer(s, cache, logger)

	// Listen before serving so a port that is already in use stops the explorer right away
	listener, err := net.Listen("tcp", s.HttpAddress)
	misc.CheckError(err, logger, misc.Fatal)
	e.coordinator.start(s.ServerAddress)
	server := &http.Server{Handler: e.routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		e.httpDone <- server.Serve(listener)
	}()

	logger.Infof("Explorer is ready at http://%s (workers connect to %s, cache in %s)", s.HttpAddress, s.ServerAddress, s.CachePath)
	return e
}

func newExplorer(s settings, cache *tileCache, logger bslogger.Logger) *Explorer {
	e := &Explorer{
		cache:    cache,
		logger:   logger,
		sessions: newSessionGuard(time.Now),
		settings: s,
		httpDone: make(chan error, 1),
	}
	e.coordinator = newCoordinator(bslogger.NewLogger("ExplorerCoordinator", bslogger.Normal, nil), s.MandelbrotSettings, e.cacheTile)
	return e
}

// RPCAddress is where workers connect
func (e *Explorer) RPCAddress() string {
	return e.settings.ServerAddress
}

// Wait blocks until the web server stops
func (e *Explorer) Wait() {
	err := <-e.httpDone
	if !errors.Is(err, http.ErrServerClosed) {
		e.logger.Fatal(err.Error())
	}
}

func (e *Explorer) cacheTile(id tileID, compressed []byte) {
	misc.CheckError(e.cache.put(id, compressed), e.logger, misc.Warning)
}

func (e *Explorer) routes() http.Handler {
	web, err := fs.Sub(webFiles, "web")
	misc.CheckError(err, e.logger, misc.Fatal)

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web))
	mux.HandleFunc("GET /api/config", e.handleConfig)
	mux.HandleFunc("POST /api/session", e.handleClaim)
	mux.HandleFunc("POST /api/session/heartbeat", e.handleHeartbeat)
	mux.HandleFunc("POST /api/session/release", e.handleRelease)
	mux.HandleFunc("POST /api/settings", e.handleSettings)
	mux.HandleFunc("POST /api/render", e.handleRender)
	return mux
}

func (e *Explorer) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, configResponse{
		Defaults:      e.settings.MandelbrotSettings,
		LevelZeroSpan: levelZeroSpan,
		Limits:        e.settings.Limits,
		PlaneLimit:    planeLimit,
		StartX:        e.settings.StartX,
		StartY:        e.settings.StartY,
		StartZoom:     e.settings.StartZoom,
		TileSize:      task.TileSize,
	})
}

func (e *Explorer) handleClaim(w http.ResponseWriter, r *http.Request) {
	id, ok := e.sessions.claim()
	if !ok {
		writeJSON(w, http.StatusConflict, errorResponse{"The explorer is already open in another tab"})
		return
	}
	writeJSON(w, http.StatusOK, sessionRequest{Session: id})
}

func (e *Explorer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req sessionRequest
	if !readJSON(w, r, &req) || !e.activeSession(w, req.Session) {
		return
	}
	e.sessions.heartbeat(req.Session)
	workers, queued, inProgress := e.coordinator.counts()
	writeJSON(w, http.StatusOK, statusResponse{CacheBytes: e.cache.size(), InProgress: inProgress, Queued: queued, Workers: workers})
}

// handleRelease is called with navigator.sendBeacon when the tab closes
func (e *Explorer) handleRelease(w http.ResponseWriter, r *http.Request) {
	var req sessionRequest
	if !readJSON(w, r, &req) {
		return
	}
	e.sessions.release(req.Session)
	w.WriteHeader(http.StatusNoContent)
}

// handleSettings normalizes the page's form the same way settings files are: Verify fills in defaults and generates
// the palette, so palette generation stays in one place, and the limits are applied.
func (e *Explorer) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if !readJSON(w, r, &req) || !e.activeSession(w, req.Session) {
		return
	}
	s := req.Settings
	s.Palette = nil // always generated from GeneratePaletteSettings
	misc.CheckError(s.Verify(), e.logger, misc.Warning)
	e.settings.Limits.clamp(&s)
	writeJSON(w, http.StatusOK, s)
}

// handleRender streams the requested tiles: cached ones straight away, then the rest as workers finish them. It ends
// when every tile is sent, when the page aborts the request, or when a newer render starts.
func (e *Explorer) handleRender(w http.ResponseWriter, r *http.Request) {
	var req renderRequest
	if !readJSON(w, r, &req) || !e.activeSession(w, req.Session) {
		return
	}
	if err := e.settings.Limits.checkRender(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{err.Error()})
		return
	}
	ctx, cancel := e.startRender(r.Context())
	defer cancel()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher := http.NewResponseController(w)
	send := func(result tileResult) bool {
		if err := writeFrame(w, result); err != nil {
			return false
		}
		return flusher.Flush() == nil
	}

	var missing []tileID
	seen := make(map[tileKey]bool, len(req.Tiles))
	for _, key := range req.Tiles {
		if seen[key] {
			continue
		}
		seen[key] = true
		id := tileID{tileKey: key, Boundary: req.Boundary, MaxIterations: req.MaxIterations, SuperSampling: req.SuperSampling}
		compressed, cached, err := e.cache.get(id)
		misc.CheckError(err, e.logger, misc.Warning)
		if !cached {
			missing = append(missing, id)
			continue
		}
		if ctx.Err() != nil || !send(tileResult{ID: id, Compressed: compressed, Cached: true}) {
			return
		}
	}
	if len(missing) == 0 {
		return
	}

	j := newJob(len(missing))
	e.coordinator.submit(j, missing)
	defer e.coordinator.cancel(j)
	for range missing {
		select {
		case result := <-j.results:
			if !send(result) {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// startRender cancels the render stream in progress, if any, since the page only ever wants its latest view
func (e *Explorer) startRender(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	e.renderMutex.Lock()
	if e.cancelRender != nil {
		e.cancelRender()
	}
	e.cancelRender = cancel
	e.renderMutex.Unlock()
	return ctx, cancel
}

// activeSession answers 409 Conflict unless id is the tab that has the explorer
func (e *Explorer) activeSession(w http.ResponseWriter, id string) bool {
	if e.sessions.is(id) {
		return true
	}
	writeJSON(w, http.StatusConflict, errorResponse{"The explorer is open in another tab"})
	return false
}

// writeFrame writes one tile of a render stream: the header described by frameHeaderSize, then the compressed samples
func writeFrame(w io.Writer, result tileResult) error {
	header := make([]byte, frameHeaderSize)
	var flags uint32
	if result.Cached {
		flags |= frameCached
	}
	binary.LittleEndian.PutUint32(header[0:], uint32(int32(result.ID.Level)))
	binary.LittleEndian.PutUint32(header[4:], flags)
	binary.LittleEndian.PutUint64(header[8:], uint64(result.ID.X))
	binary.LittleEndian.PutUint64(header[16:], uint64(result.ID.Y))
	binary.LittleEndian.PutUint32(header[24:], uint32(len(result.Compressed)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(result.Compressed)
	return err
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(into); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid request: " + err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
