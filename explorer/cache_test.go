package explorer

import (
	"bytes"
	"testing"
	"time"
)

// newTestCache opens a cache in a temporary folder with a clock that advances one second per call, so the order tiles
// were used in is unambiguous
func newTestCache(t *testing.T, maxBytes int64) *tileCache {
	t.Helper()
	c, err := openTileCache(t.TempDir(), maxBytes)
	if err != nil {
		t.Fatalf("openTileCache: %s", err)
	}
	t.Cleanup(func() { c.close() })
	clock := time.Unix(1_000_000, 0)
	c.now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}
	return c
}

func mustGet(t *testing.T, c *tileCache, id tileID) ([]byte, bool) {
	t.Helper()
	data, ok, err := c.get(id)
	if err != nil {
		t.Fatalf("get %s: %s", id.tileKey, err)
	}
	return data, ok
}

func mustPut(t *testing.T, c *tileCache, id tileID, data []byte) {
	t.Helper()
	if err := c.put(id, data); err != nil {
		t.Fatalf("put %s: %s", id.tileKey, err)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	c := newTestCache(t, 1<<20)
	id := tileID{tileKey: tileKey{-2, -1, 3}, Boundary: 1000, MaxIterations: 5000, SuperSampling: 4}
	if _, ok := mustGet(t, c, id); ok {
		t.Fatal("an empty cache reported a hit")
	}
	data := []byte("some compressed samples")
	mustPut(t, c, id, data)
	got, ok := mustGet(t, c, id)
	if !ok || !bytes.Equal(got, data) {
		t.Errorf("get = %q, %v; want %q", got, ok, data)
	}
	if c.size() != int64(len(data)) {
		t.Errorf("size = %d, want %d", c.size(), len(data))
	}
	// Storing the same tile again changes nothing
	mustPut(t, c, id, data)
	if c.size() != int64(len(data)) {
		t.Errorf("size after storing a tile twice = %d, want %d", c.size(), len(data))
	}
}

// Every setting that changes the samples is part of the key
func TestCacheKeyIncludesSettings(t *testing.T) {
	c := newTestCache(t, 1<<20)
	base := tileID{tileKey: tileKey{5, 10, -7}, Boundary: 1000, MaxIterations: 1000, SuperSampling: 2}
	mustPut(t, c, base, []byte("base"))

	variants := map[string]func(id *tileID){
		"level":          func(id *tileID) { id.Level++ },
		"x":              func(id *tileID) { id.X++ },
		"y":              func(id *tileID) { id.Y++ },
		"boundary":       func(id *tileID) { id.Boundary = 100 },
		"max iterations": func(id *tileID) { id.MaxIterations = 2000 },
		"super sampling": func(id *tileID) { id.SuperSampling = 1 },
	}
	for name, change := range variants {
		id := base
		change(&id)
		if _, ok := mustGet(t, c, id); ok {
			t.Errorf("a tile with a different %s was found in the cache", name)
		}
	}
}

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := newTestCache(t, 100)
	tile := func(x int64) tileID {
		return tileID{tileKey: tileKey{1, x, 0}, Boundary: 1000, MaxIterations: 100, SuperSampling: 1}
	}
	data := bytes.Repeat([]byte{1}, 30)
	for x := range int64(3) {
		mustPut(t, c, tile(x), data) // 90 bytes, under the limit
	}
	mustGet(t, c, tile(0)) // tile 1 is now the least recently used

	mustPut(t, c, tile(3), data) // 120 bytes: trimmed to at most 90
	if c.size() > 90 {
		t.Errorf("size after eviction = %d, want at most 90", c.size())
	}
	for x, want := range []bool{true, false, true, true} {
		if _, ok := mustGet(t, c, tile(int64(x))); ok != want {
			t.Errorf("tile %d cached = %v, want %v", x, ok, want)
		}
	}
}

// The size total survives reopening the cache
func TestCacheReopen(t *testing.T) {
	folder := t.TempDir()
	c, err := openTileCache(folder, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	id := tileID{tileKey: tileKey{0, 0, 0}, Boundary: 4, MaxIterations: 10, SuperSampling: 1}
	if err = c.put(id, []byte("12345")); err != nil {
		t.Fatal(err)
	}
	c.close()

	c, err = openTileCache(folder, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	if c.size() != 5 {
		t.Errorf("size after reopening = %d, want 5", c.size())
	}
	if _, ok, _ := c.get(id); !ok {
		t.Error("the tile was lost when the cache was reopened")
	}
}

func TestCompressSamplesRoundTrip(t *testing.T) {
	samples := bytes.Repeat([]byte{0, 0, 0x7a, 0x44}, 4096)
	compressed, err := compressSamples(samples)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) >= len(samples) {
		t.Errorf("compressed %d repetitive bytes into %d", len(samples), len(compressed))
	}
	got, err := decompressSamples(compressed)
	if err != nil || !bytes.Equal(got, samples) {
		t.Errorf("decompressed samples differ (error %v)", err)
	}
}
