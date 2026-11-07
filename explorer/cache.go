package explorer

import (
	"bytes"
	"compress/flate"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure Go, so no cgo is needed
)

// cacheFileName is the SQLite database inside the explorer's CachePath
const cacheFileName = "cache.db"

// evictTo is the fraction of the size limit the cache is trimmed down to once it goes over, so a full cache does not
// evict on every insert
const evictTo = 0.9

// tileCache stores computed tiles, compressed, in SQLite. Colors are applied in the browser, so a tile is keyed only
// by its position and the settings that change its iteration counts (tileID).
type tileCache struct {
	db       *sql.DB
	maxBytes int64
	now      func() time.Time

	mutex      sync.Mutex // guards totalBytes and serializes eviction
	totalBytes int64
}

func openTileCache(folder string, maxBytes int64) (*tileCache, error) {
	if err := os.MkdirAll(folder, os.ModePerm); err != nil {
		return nil, err
	}
	// The pragmas apply to every connection the pool opens. WAL lets reads continue during a write, and the busy
	// timeout makes a writer wait for another instead of failing.
	dsn := "file:" + filepath.ToSlash(filepath.Join(folder, cacheFileName)) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serializes access, which is plenty for one browser tab and avoids SQLITE_BUSY between writers
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tiles (
			level          INTEGER NOT NULL,
			x              INTEGER NOT NULL,
			y              INTEGER NOT NULL,
			max_iterations INTEGER NOT NULL,
			boundary       REAL    NOT NULL,
			super_sampling INTEGER NOT NULL,
			data           BLOB    NOT NULL,
			size           INTEGER NOT NULL,
			last_used      INTEGER NOT NULL,
			PRIMARY KEY (level, x, y, max_iterations, boundary, super_sampling)
		);
		CREATE INDEX IF NOT EXISTS tiles_last_used ON tiles (last_used);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to create the cache table: %w", err)
	}

	c := &tileCache{db: db, maxBytes: maxBytes, now: time.Now}
	if err = db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM tiles`).Scan(&c.totalBytes); err != nil {
		db.Close()
		return nil, err
	}
	return c, nil
}

func (c *tileCache) close() error {
	return c.db.Close()
}

// get returns the compressed samples of a cached tile and marks it as recently used, or ok false if it is not cached
func (c *tileCache) get(id tileID) (compressed []byte, ok bool, err error) {
	where := `level = ? AND x = ? AND y = ? AND max_iterations = ? AND boundary = ? AND super_sampling = ?`
	args := []any{id.Level, id.X, id.Y, id.MaxIterations, id.Boundary, id.SuperSampling}
	err = c.db.QueryRow(`SELECT data FROM tiles WHERE `+where, args...).Scan(&compressed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	_, err = c.db.Exec(`UPDATE tiles SET last_used = ? WHERE `+where, append([]any{c.now().UnixNano()}, args...)...)
	return compressed, true, err
}

// put stores a tile, then evicts the least recently used tiles if the cache is over its size limit. A tile that is
// already cached is left alone, since the same tile always computes the same samples.
func (c *tileCache) put(id tileID, compressed []byte) error {
	result, err := c.db.Exec(`INSERT INTO tiles (level, x, y, max_iterations, boundary, super_sampling, data, size, last_used)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		id.Level, id.X, id.Y, id.MaxIterations, id.Boundary, id.SuperSampling, compressed, len(compressed), c.now().UnixNano())
	if err != nil {
		return err
	}
	if inserted, _ := result.RowsAffected(); inserted == 0 {
		return nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.totalBytes += int64(len(compressed))
	if c.totalBytes <= c.maxBytes {
		return nil
	}
	return c.evict(int64(float64(c.maxBytes) * evictTo))
}

// evict deletes the least recently used tiles until the cache holds at most target bytes. Must hold mutex.
func (c *tileCache) evict(target int64) error {
	for c.totalBytes > target {
		var rowID, size int64
		err := c.db.QueryRow(`SELECT rowid, size FROM tiles ORDER BY last_used LIMIT 1`).Scan(&rowID, &size)
		if errors.Is(err, sql.ErrNoRows) {
			c.totalBytes = 0
			return nil
		}
		if err != nil {
			return err
		}
		if _, err = c.db.Exec(`DELETE FROM tiles WHERE rowid = ?`, rowID); err != nil {
			return err
		}
		c.totalBytes -= size
	}
	return nil
}

// size is the total size of the cached tiles' data
func (c *tileCache) size() int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.totalBytes
}

// compressSamples compresses a tile's encoded samples with raw deflate, which the browser decompresses with
// DecompressionStream("deflate-raw"). The same bytes are cached and streamed, so a tile is only compressed once.
func compressSamples(samples []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(samples); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func decompressSamples(compressed []byte) ([]byte, error) {
	return io.ReadAll(flate.NewReader(bytes.NewReader(compressed)))
}
