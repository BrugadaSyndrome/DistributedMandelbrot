package explorer

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// sessionTimeout is how long a session lasts without a heartbeat. The page sends one every 10 seconds, but browsers
// slow timers in background tabs to about once a minute, so the timeout leaves room for that. A tab that closes
// normally releases its session at once.
const sessionTimeout = 2 * time.Minute

// sessionGuard lets one browser tab use the explorer at a time. The tab that claims it first keeps it until it
// releases it or stops sending heartbeats; any other tab is turned away until then.
type sessionGuard struct {
	mutex    sync.Mutex
	active   string
	lastSeen time.Time
	now      func() time.Time
}

func newSessionGuard(now func() time.Time) *sessionGuard {
	return &sessionGuard{now: now}
}

// claim starts a new session if none is active, returning its ID, or ok false if another tab has it
func (g *sessionGuard) claim() (id string, ok bool) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.activeLocked() {
		return "", false
	}
	random := make([]byte, 16)
	_, _ = rand.Read(random) // never returns an error
	g.active = hex.EncodeToString(random)
	g.lastSeen = g.now()
	return g.active, true
}

// heartbeat keeps id's session alive, or reports false if id is not the active session
func (g *sessionGuard) heartbeat(id string) bool {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if !g.isLocked(id) {
		return false
	}
	g.lastSeen = g.now()
	return true
}

// release ends id's session so another tab can claim the explorer
func (g *sessionGuard) release(id string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.isLocked(id) {
		g.active = ""
	}
}

// is reports whether id is the active session
func (g *sessionGuard) is(id string) bool {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.isLocked(id)
}

func (g *sessionGuard) isLocked(id string) bool {
	return id != "" && id == g.active && g.activeLocked()
}

func (g *sessionGuard) activeLocked() bool {
	return g.active != "" && g.now().Sub(g.lastSeen) < sessionTimeout
}
