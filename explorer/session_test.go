package explorer

import (
	"testing"
	"time"
)

// fakeClock is a clock tests move by hand
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func TestSessionFirstTabKeepsTheExplorer(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	g := newSessionGuard(clock.Now)

	first, ok := g.claim()
	if !ok || first == "" {
		t.Fatal("the first claim failed")
	}
	if _, ok = g.claim(); ok {
		t.Fatal("a second tab claimed the explorer while the first had it")
	}
	if !g.is(first) || g.is("someone else") || g.is("") {
		t.Error("only the first tab's session should be active")
	}

	// Heartbeats keep it alive past the timeout
	for range 5 {
		clock.now = clock.now.Add(sessionTimeout / 2)
		if !g.heartbeat(first) {
			t.Fatal("a heartbeat from the active tab was refused")
		}
	}
	if _, ok = g.claim(); ok {
		t.Error("a second tab claimed the explorer while the first kept sending heartbeats")
	}
	if g.heartbeat("someone else") {
		t.Error("a heartbeat from another tab was accepted")
	}
}

func TestSessionRelease(t *testing.T) {
	g := newSessionGuard(time.Now)
	first, _ := g.claim()
	g.release("someone else")
	if !g.is(first) {
		t.Fatal("another tab released the active session")
	}
	g.release(first)
	if g.is(first) {
		t.Error("the session is still active after its release")
	}
	second, ok := g.claim()
	if !ok || second == first {
		t.Errorf("claim after a release = %q, %v; want a new session", second, ok)
	}
}

func TestSessionExpires(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	g := newSessionGuard(clock.Now)
	first, _ := g.claim()

	clock.now = clock.now.Add(sessionTimeout - time.Second)
	if _, ok := g.claim(); ok {
		t.Fatal("the session expired early")
	}
	clock.now = clock.now.Add(time.Second)
	if g.is(first) || g.heartbeat(first) {
		t.Error("an expired session is still active")
	}
	if _, ok := g.claim(); !ok {
		t.Error("a new tab could not claim the explorer after the session expired")
	}
}
