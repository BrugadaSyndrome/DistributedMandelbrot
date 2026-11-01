# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and run

Go module `DistributedMandelbrot` (Go 1.27, per `go.mod`). Tests use only the standard `testing` package (no assertion libraries).

```bash
go build            # produces DistributedMandelbrot(.exe)
go vet ./...
go test ./...
go test ./mandelbrot/ -run TestPeriodicityCheckExitsEarly -v   # single test
go test ./mandelbrot/ -run '^$' -bench .                        # benchmarks only
```

`go test -race` needs cgo (`CGO_ENABLED=1` plus a C compiler), which is not set up on the Windows dev machine.

Testing conventions: tests live next to the code in the same package (internal tests), so unexported types like `generatePaletteSettings`, `transitionSettings` and `escapeTime` can be tested directly. Prefer table-driven tests. `mandelbrot.escapeTime` returns the loop count alongside the result as a test seam; `TestEscapeTimeMatchesNaive` guards that optimizations never change pixel output. `NewCoordinator`/`NewWorker` start RPC servers and goroutines, so put new logic in pure functions or methods (like `transitionSettings.frameCount`/`frame`) that can be tested directly.

A run needs two processes; start the coordinator first, then the worker(s):

```bash
./DistributedMandelbrot -mode=coordinator -settings=settings_coordinator.json
./DistributedMandelbrot -mode=worker -settings=settings_worker.json -workers=5
```

`run_coordinator.cmd` / `run_worker.cmd` are the Windows equivalents. `-workers` (default 2) is how many worker instances one worker process spins up, each with its own RPC server on a free port.

Output goes to `<SavePath>/<RunName>/` (defaults: cwd and `run_<timestamp>`): one zero-padded JPEG per frame, a copy of the settings JSON, `coordinator.log`, and `movie.mp4` if `GenerateMovie` is true and `ffmpeg` is on PATH (otherwise it is silently disabled).

## Architecture

Two roles in one binary, selected in `main.go`. All communication is bidirectional net/rpc over TCP via the external `github.com/BrugadaSyndrome/multirpc` module; logging uses `github.com/BrugadaSyndrome/bslogger`. Both are the author's own modules (split out of this repo), so API changes there show up here as dependency bumps.

**Coordinator** (`coordinator/`) is an RPC server. On startup it:
1. Computes the frame count for each `TransitionSettings` entry with `transitionSettings.frameCount()` and the total task count from `TaskGeneration`. The coordinator shuts down when `taskIngestedCount == taskCount`, so **any change to how tasks are generated must keep that count calculation in sync**.
2. `generateTasks` goroutine walks each transition frame by frame, getting each frame's center and magnification from `transitionSettings.frame(i)`, and pushes `task.Task`s into the buffered `tasksTodo` channel. `tasksTodo` is never closed, because tasks from departed workers are pushed back into it. Frame `i` is at `start × step^i` (or `÷` when zooming out); the first frame is exactly the start values and the last frame reaches the end magnification (exactly, or less than one step past) and exactly the end center. Consecutive transitions that share an endpoint therefore render that boundary frame twice.
3. `ingestTasks` goroutine reads finished tasks from `tasksDone`, paints pixels into a per-frame `image.RGBA` (`images` map keyed by frame number), and writes the JPEG and frees memory when that frame's `PixelsLeft` reaches 0.

RPC methods exposed to workers: `RegisterWorker`, `DeRegisterWorker`, `GetTask`, `ReturnTask`, `GetMandelbrotSettings`, `RollCall`. The end of work is "every task ingested", not "every task handed out":
- `GetTask` blocks until a task is queued or `allTasksDone` is closed (by `ingestTasks` once `taskIngestedCount == taskCount`), then returns `task.NoMoreTasksMessage`. RPC errors arrive as strings, so the worker compares against that shared constant.
- The coordinator tracks tasks per worker in `tasksHandedOut`. On deregistration (or a missed roll call, checked every minute) unreturned tasks are requeued in a background goroutine (`requeue`), so callers never block on a full channel or while holding the mutex.
- `ReturnTask` only accepts a task still recorded as handed out to the returning worker and removes it from `tasksHandedOut`; late copies from removed workers and repeat returns are dropped, which keeps the exact-count shutdown correct.
- `GetTask`/`DeRegisterWorker` return errors for unknown workers (a worker dropped by roll call may still call them). A worker stops itself on any `GetTask` error rather than exiting the whole process.

Worker-facing coordinator methods are tested in `coordinator_test.go` by building a `Coordinator` struct directly (`newTestCoordinator`/`addTestWorker`) without starting servers or goroutines.

**Worker** (`worker/`) is both an RPC server (for the coordinator's `Worker.RollCall`) and a client. It registers, fetches the `mandelbrot.Settings` from the coordinator (so render settings live only in the coordinator's JSON), then loops `GetTask` → compute every `Coordinate` → `ReturnTask` until told there is no more work.

**Shared packages:**
- `task/` — the RPC payload. A `Task` carries a list of `Coordinate`s (pixel row/column plus that frame's center and magnification) and accumulates `Pixel` results. `Generation` enum (`Row`, `Column`, `Image`, `Grid`) controls task granularity; it is set by the integer `TaskGeneration` in the coordinator JSON.
- `mandelbrot/` — pure computation: pixel→complex mapping (scaled by the image's shorter side), escape-time with periodicity check, smooth coloring, grid super-sampling, palette generation from `GeneratePaletteSettings`.
- `misc/` — helpers: `CheckError(err, logger, severity)` is the pervasive error-handling idiom (Fatal exits), plus file IO, lerp/easing, free-port and local-IP discovery, and the empty `Nothing` type used for RPC args/replies.

## Conventions

- Settings structs are loaded from JSON and then normalized by a `Verify()` method that replaces missing/out-of-range values with defaults rather than erroring. Add new settings the same way (exported field + default in `Verify`).
- Exported struct fields are what crosses RPC (gob encoding); unexported fields like `logger` are not sent.
- When no address is configured, both sides default to the first non-loopback IPv4 on port 51000.

## Gotchas

- `.gitignore`'s `run_*` pattern matches the tracked `run_*.cmd` scripts, so a new `run_*` file needs `git add -f`.
- Grid generation splits each frame into `task.GridDivisions`² cells (zero-based indices; the last row/column of cells absorbs the remainder). The coordinator's Grid `taskCount` is derived from the same constant.
- `NewCoordinator` returns `*Coordinator` because the RPC server and background goroutines must share one instance (it holds a `sync.Mutex`). `NewWorker` still returns `Worker` by value while its RPC server is registered with a pointer to the local, so `main.go`'s copies differ from the instance actually serving RPCs.
