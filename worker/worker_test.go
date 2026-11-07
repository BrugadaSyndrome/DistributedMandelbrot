package worker

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/task"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
	"github.com/BrugadaSyndrome/multirpc"
)

// newTestWorker builds a Worker with a running RPC server on a free local port and a client that is never connected,
// without registering with a coordinator or starting any background goroutines
func newTestWorker(t *testing.T) *Worker {
	t.Helper()
	w := newWorker(bslogger.NewLogger("WorkerTest", bslogger.Normal, nil))
	w.ServerClient = multirpc.NewTcpServerClient(w, "127.0.0.1:0", "WorkerTestServer", "127.0.0.1:1", "CoordinatorTestClient")
	if err := w.ServerClient.Server.Run(); err != nil {
		t.Fatalf("unable to start worker server: %s", err)
	}
	return w
}

func waitFor(t *testing.T, description string, wait func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

// Both the roll call ticker and processTasks shut the worker down, and stopping the RPC server twice panics
func TestShutdownIsSafeToCallRepeatedly(t *testing.T) {
	w := newTestWorker(t)

	var callers sync.WaitGroup
	for range 10 {
		callers.Go(func() {
			w.shutdown()
		})
	}
	callers.Wait()
	w.shutdown()

	waitFor(t, "the worker server to stop", w.ServerClient.Server.Wait)
	select {
	case <-w.stopped:
	default:
		t.Error("stopped should be closed after shutdown")
	}
}

// The roll call ticker must not outlive the worker, otherwise it later fails a roll call and shuts down again
func TestTickersExitAfterShutdown(t *testing.T) {
	w := newTestWorker(t)

	exited := make(chan struct{})
	go func() {
		w.tickers()
		close(exited)
	}()

	w.shutdown()
	waitFor(t, "tickers to exit", func() { <-exited })
}

// status is read by the heartbeat ticker while processTasks counts completed tasks
func TestStatusWithConcurrentUpdates(t *testing.T) {
	w := newWorker(bslogger.NewLogger("WorkerTest", bslogger.Normal, nil))

	var updaters sync.WaitGroup
	for range 50 {
		updaters.Go(func() {
			for range 1000 {
				w.tasksCompleted.Add(1)
				_ = w.status()
			}
		})
	}
	updaters.Wait()

	if got, want := w.status(), "Tasks [Completed: 50000]"; got != want {
		t.Errorf("status() = %q, want %q", got, want)
	}
}

// render must produce the same color the mandelbrot package computes for every pixel, packed row by row
func TestRenderFillsPixelsRowByRow(t *testing.T) {
	// A multi-color palette and a zoomed out frame give every pixel a different color, so the order is observable
	var palette []color.RGBA
	for i := range 8 {
		palette = append(palette, color.RGBA{R: uint8(30 * i), G: uint8(255 - 30*i), B: uint8(60 * (i % 4)), A: 255})
	}
	settings := mandelbrot.Settings{Width: 40, Height: 30, MaxIterations: 200, SuperSampling: 2, Palette: palette}
	_ = settings.Verify()
	w := newWorker(bslogger.NewLogger("WorkerTest", bslogger.Normal, nil))
	w.mandelbrot = mandelbrot.NewMandelbrot(settings)

	frame := task.Frame{CenterX: -0.5, CenterY: 0, Magnification: 0.5}
	bounds := image.Rect(3, 2, 20, 9) // not square, so swapping rows and columns changes the layout
	todo := task.NewTask(0, 1, frame, bounds)
	w.render(&todo)

	if !todo.HasValidResult() {
		t.Fatalf("render produced %d bytes, want %d", len(todo.Pixels), todo.ResultSize())
	}
	distinct := make(map[[4]byte]bool)
	for i := 0; i < len(todo.Pixels); i += task.BytesPerPixel {
		distinct[[4]byte(todo.Pixels[i:i+task.BytesPerPixel])] = true
	}
	if len(distinct) < 4 {
		t.Fatalf("the region has only %d distinct colors, too few to check the pixel order", len(distinct))
	}

	i := 0
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		for column := bounds.Min.X; column < bounds.Max.X; column++ {
			want := w.mandelbrot.GetColorMultiple(w.mandelbrot.EscapeTimeMultiple(w.mandelbrot.GetPointsToCalculate(frame, column, row)))
			got := todo.Pixels[i : i+task.BytesPerPixel]
			if got[0] != want.R || got[1] != want.G || got[2] != want.B || got[3] != want.A {
				t.Fatalf("pixel (%d, %d) = %v, want %v", column, row, got, want)
			}
			i += task.BytesPerPixel
		}
	}
}

func TestRenderEmptyBounds(t *testing.T) {
	w := newWorker(bslogger.NewLogger("WorkerTest", bslogger.Normal, nil))
	todo := task.NewTask(0, 1, task.Frame{}, image.Rectangle{})
	w.render(&todo)
	if len(todo.Pixels) != 0 || !todo.HasValidResult() {
		t.Errorf("render of an empty task produced %d bytes", len(todo.Pixels))
	}
}

// A tile task gets iteration samples from mandelbrot.TileSamples rather than colors from the worker's movie settings
func TestRenderTileReturnsSamples(t *testing.T) {
	w := newWorker(bslogger.NewLogger("WorkerTest", bslogger.Normal, nil))
	tile := &task.Tile{Boundary: 1000, MaxIterations: 100, OriginX: -2, OriginY: -1.5, PixelSize: 3.0 / task.TileSize, SuperSampling: 2}
	todo := task.Task{Bounds: image.Rect(0, 0, task.TileSize, task.TileSize), Tile: tile}
	w.render(&todo)

	if !todo.HasValidResult() {
		t.Fatalf("render produced %d bytes, want %d", len(todo.Pixels), todo.ResultSize())
	}
	want := mandelbrot.TileSamples(*tile, todo.Bounds)
	got := task.DecodeSamples(todo.Pixels)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
}
