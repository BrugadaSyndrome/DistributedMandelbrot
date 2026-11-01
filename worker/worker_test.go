package worker

import (
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
		callers.Add(1)
		go func() {
			defer callers.Done()
			w.shutdown()
		}()
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
		updaters.Add(1)
		go func() {
			defer updaters.Done()
			for range 1000 {
				w.tasksCompleted.Add(1)
				_ = w.status()
			}
		}()
	}
	updaters.Wait()

	if got, want := w.status(), "Tasks [Completed: 50000]"; got != want {
		t.Errorf("status() = %q, want %q", got, want)
	}
}
