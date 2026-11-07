package explorer

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/task"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/BrugadaSyndrome/bslogger"
)

// newTestExplorer builds an Explorer with a temporary cache and serves its routes, without starting the RPC server
func newTestExplorer(t *testing.T) (*Explorer, *httptest.Server) {
	t.Helper()
	s := settings{
		logger:        bslogger.NewLogger("ExplorerTest", bslogger.Normal, nil),
		HttpAddress:   "127.0.0.1:0",
		ServerAddress: "127.0.0.1:0",
		Limits:        limits{MaxZoomLevel: 30},
		StartX:        -0.75,
		StartZoom:     2.5,
	}
	if err := s.Verify(); err != nil {
		t.Fatal(err)
	}
	e := newExplorer(s, newTestCache(t, 1<<30), s.logger)
	server := httptest.NewServer(e.routes())
	t.Cleanup(server.Close)
	return e, server
}

func post(t *testing.T, server *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+path, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("POST %s: %s", path, err)
	}
	return response
}

func decode[T any](t *testing.T, response *http.Response) T {
	t.Helper()
	defer response.Body.Close()
	var value T
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatalf("decoding the response: %s", err)
	}
	return value
}

func claim(t *testing.T, server *httptest.Server) string {
	t.Helper()
	response := post(t, server, "/api/session", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("claim status = %d, want 200", response.StatusCode)
	}
	return decode[sessionRequest](t, response).Session
}

// frame is one tile read back from a render stream
type frame struct {
	key     tileKey
	cached  bool
	samples []float32
}

func readFrames(t *testing.T, stream io.Reader) []frame {
	t.Helper()
	var frames []frame
	header := make([]byte, frameHeaderSize)
	for {
		if _, err := io.ReadFull(stream, header); err == io.EOF {
			return frames
		} else if err != nil {
			t.Fatalf("reading a frame header: %s", err)
		}
		payload := make([]byte, binary.LittleEndian.Uint32(header[24:]))
		if _, err := io.ReadFull(stream, payload); err != nil {
			t.Fatalf("reading a frame: %s", err)
		}
		decompressed, err := decompressSamples(payload)
		if err != nil {
			t.Fatalf("decompressing a frame: %s", err)
		}
		frames = append(frames, frame{
			key: tileKey{
				Level: int(int32(binary.LittleEndian.Uint32(header[0:]))),
				X:     int64(binary.LittleEndian.Uint64(header[8:])),
				Y:     int64(binary.LittleEndian.Uint64(header[16:])),
			},
			cached:  binary.LittleEndian.Uint32(header[4:])&frameCached != 0,
			samples: task.DecodeSamples(decompressed),
		})
	}
}

// cacheTestTile stores samples for key in the explorer's cache
func cacheTestTile(t *testing.T, e *Explorer, id tileID, samples []float32) {
	t.Helper()
	compressed, err := compressSamples(task.EncodeSamples(samples))
	if err != nil {
		t.Fatal(err)
	}
	mustPut(t, e.cache, id, compressed)
}

func TestConfig(t *testing.T) {
	_, server := newTestExplorer(t)
	response, err := http.Get(server.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	config := decode[configResponse](t, response)
	if config.Limits.MaxIterations != 10000 || config.Limits.MaxSuperSampling != 4 || config.Limits.MaxZoomLevel != 30 {
		t.Errorf("limits = %+v, want 10000 iterations, 4x super sampling and level 30", config.Limits)
	}
	if config.StartX != -0.75 || config.StartZoom != 2.5 || config.TileSize != 256 || config.LevelZeroSpan != 4 {
		t.Errorf("config = %+v, want the start view and tile geometry", config)
	}
	if len(config.Defaults.Palette) == 0 {
		t.Error("the defaults should include the generated palette")
	}
}

func TestPageIsServed(t *testing.T) {
	_, server := newTestExplorer(t)
	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>") {
		t.Errorf("GET / = %d, want the explorer page", response.StatusCode)
	}
}

func TestSecondTabIsTurnedAway(t *testing.T) {
	_, server := newTestExplorer(t)
	first := claim(t, server)

	response := post(t, server, "/api/session", nil)
	if response.StatusCode != http.StatusConflict {
		t.Errorf("second claim status = %d, want 409", response.StatusCode)
	}
	response.Body.Close()

	for _, path := range []string{"/api/session/heartbeat", "/api/settings", "/api/render"} {
		response = post(t, server, path, sessionRequest{Session: "not the active tab"})
		if response.StatusCode != http.StatusConflict {
			t.Errorf("%s from another tab: status %d, want 409", path, response.StatusCode)
		}
		response.Body.Close()
	}

	response = post(t, server, "/api/session/heartbeat", sessionRequest{Session: first})
	status := decode[statusResponse](t, response)
	if response.StatusCode != http.StatusOK || status.Workers != 0 {
		t.Errorf("heartbeat from the active tab = %d %+v, want 200 with no workers", response.StatusCode, status)
	}

	response = post(t, server, "/api/session/release", sessionRequest{Session: first})
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Errorf("release status = %d, want 204", response.StatusCode)
	}
	claim(t, server) // fails the test unless the explorer is free again
}

func TestSettingsAreNormalized(t *testing.T) {
	_, server := newTestExplorer(t)
	session := claim(t, server)
	form := mandelbrot.Settings{
		Boundary:      0, // filled in by Verify
		MaxIterations: 100000000,
		SuperSampling: 9,
		Palette:       []color.RGBA{{R: 1, G: 2, B: 3, A: 255}}, // ignored: the palette is always generated
	}
	if err := json.Unmarshal([]byte(`[{"StartColor":{"R":0,"G":0,"B":0,"A":255},"EndColor":{"R":255,"G":255,"B":255,"A":255},"NumberColors":5}]`), &form.GeneratePaletteSettings); err != nil {
		t.Fatal(err)
	}
	response := post(t, server, "/api/settings", settingsRequest{Session: session, Settings: form})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	got := decode[mandelbrot.Settings](t, response)
	if got.MaxIterations != 10000 || got.SuperSampling != 4 || got.Boundary != 100 {
		t.Errorf("normalized MaxIterations %d, SuperSampling %d, Boundary %v; want 10000, 4 and 100", got.MaxIterations, got.SuperSampling, got.Boundary)
	}
	if len(got.Palette) != 5 || got.Palette[0].R != 0 || got.Palette[4].R != 204 {
		t.Errorf("palette = %v, want 5 colors from black toward white", got.Palette)
	}
}

func TestRenderRejectsBadRequests(t *testing.T) {
	_, server := newTestExplorer(t)
	session := claim(t, server)
	response := post(t, server, "/api/render", renderRequest{Session: session, Boundary: 1000, MaxIterations: 100000000, SuperSampling: 1,
		Tiles: []tileKey{{2, 0, 0}}})
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status for too many iterations = %d, want 400", response.StatusCode)
	}
	response.Body.Close()

	response, err := http.Post(server.URL+"/api/render", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status for invalid JSON = %d, want 400", response.StatusCode)
	}
}

// Cached tiles are streamed first, then computed ones as workers return them
func TestRenderStreamsCachedThenComputedTiles(t *testing.T) {
	e, server := newTestExplorer(t)
	session := claim(t, server)
	addTestWorker(e.coordinator, "w1")

	request := renderRequest{Session: session, Boundary: 1000, MaxIterations: 100, SuperSampling: 1,
		Tiles: []tileKey{{3, -2, 0}, {3, -1, 0}, {3, -2, 0}}} // the repeat is ignored
	cachedSamples := make([]float32, task.TileSize*task.TileSize)
	for i := range cachedSamples {
		cachedSamples[i] = float32(i % 13)
	}
	cacheTestTile(t, e, tileID{tileKey: tileKey{3, -2, 0}, Boundary: 1000, MaxIterations: 100, SuperSampling: 1}, cachedSamples)

	// Play the worker: compute the one missing tile once it is queued
	go func() {
		var todo task.Task
		if e.coordinator.GetTask("w1", &todo) != nil {
			return
		}
		todo.Pixels = task.EncodeSamples(mandelbrot.TileSamples(*todo.Tile, todo.Bounds))
		_ = e.coordinator.ReturnTask(todo, nil)
	}()

	response := post(t, server, "/api/render", request)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	frames := readFrames(t, response.Body)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[0].key != (tileKey{3, -2, 0}) || !frames[0].cached || !slices.Equal(frames[0].samples, cachedSamples) {
		t.Errorf("first frame = %v cached %v, want the cached tile 3/-2/0", frames[0].key, frames[0].cached)
	}
	computed := tileID{tileKey: tileKey{3, -1, 0}, Boundary: 1000, MaxIterations: 100, SuperSampling: 1}
	want := mandelbrot.TileSamples(*computed.task(0).Tile, computed.task(0).Bounds)
	if frames[1].key != computed.tileKey || frames[1].cached || !slices.Equal(frames[1].samples, want) {
		t.Errorf("second frame = %v cached %v, want the computed tile 3/-1/0", frames[1].key, frames[1].cached)
	}
	// The computed tile is now cached too
	if _, ok := mustGet(t, e.cache, computed); !ok {
		t.Error("the computed tile was not cached")
	}
}

// waitUntil polls condition until it holds or a second passes
func waitUntil(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAbortedRenderDropsItsQueuedTiles(t *testing.T) {
	e, server := newTestExplorer(t)
	session := claim(t, server)
	request := renderRequest{Session: session, Boundary: 1000, MaxIterations: 100, SuperSampling: 1,
		Tiles: []tileKey{{4, 0, 0}, {4, 1, 0}, {4, 2, 0}}}

	ctx, abort := context.WithCancel(context.Background())
	encoded, _ := json.Marshal(request)
	httpRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/render", bytes.NewReader(encoded))
	go func() {
		if response, err := http.DefaultClient.Do(httpRequest); err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()

	waitUntil(t, "the tiles are queued", func() bool { return len(queueOf(e.coordinator)) == 3 })
	abort()
	waitUntil(t, "the aborted render's tiles leave the queue", func() bool { return len(queueOf(e.coordinator)) == 0 })
}

// The page only ever wants its latest view, so a new render ends the one before it
func TestNewRenderEndsThePreviousOne(t *testing.T) {
	e, server := newTestExplorer(t)
	session := claim(t, server)
	first := renderRequest{Session: session, Boundary: 1000, MaxIterations: 100, SuperSampling: 1, Tiles: []tileKey{{4, 0, 0}}}

	done := make(chan struct{})
	go func() {
		encoded, _ := json.Marshal(first)
		if response, err := http.Post(server.URL+"/api/render", "application/json", bytes.NewReader(encoded)); err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		close(done)
	}()
	waitUntil(t, "the first render's tile is queued", func() bool { return len(queueOf(e.coordinator)) == 1 })

	// Everything the second render wants is cached, so it finishes at once
	second := renderRequest{Session: session, Boundary: 1000, MaxIterations: 100, SuperSampling: 1, Tiles: []tileKey{{4, 5, 5}}}
	cacheTestTile(t, e, tileID{tileKey: tileKey{4, 5, 5}, Boundary: 1000, MaxIterations: 100, SuperSampling: 1}, make([]float32, 16))
	response := post(t, server, "/api/render", second)
	if frames := readFrames(t, response.Body); len(frames) != 1 {
		t.Errorf("second render sent %d frames, want 1", len(frames))
	}
	response.Body.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the first render kept streaming after a newer one started")
	}
	waitUntil(t, "the first render's tile leaves the queue", func() bool { return len(queueOf(e.coordinator)) == 0 })
}
