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

Testing conventions: tests live next to the code in the same package (internal tests), so unexported types like `generatePaletteSettings`, `transitionSettings` and `escapeTime` can be tested directly. Prefer table-driven tests. `mandelbrot.escapeTime` returns the loop count alongside the result as a test seam; `TestEscapeTimeMatchesNaive` guards that optimizations never change pixel output. `NewCoordinator`/`NewWorker` start RPC servers and goroutines, so put new logic in pure functions or methods (like `transitionSettings.frameCount`/`frame`) that can be tested directly. `worker_test.go`'s `newTestWorker` builds a `Worker` with a real RPC server on `127.0.0.1:0` and an unconnected client, for testing lifecycle code without a coordinator.

A run needs two processes; start the coordinator first, then the worker(s):

```bash
./DistributedMandelbrot -mode=coordinator -settings=settings_coordinator.json
./DistributedMandelbrot -mode=worker -settings=settings_worker.json -workers=5
```

`run_coordinator.cmd` / `run_worker.cmd` are the Windows equivalents. `-workers` (default 2) is how many worker instances one worker process spins up, each with its own RPC server on a free port.

Output goes to `<SavePath>/<RunName>/` (defaults: cwd and `run_<24-hour timestamp>`): one zero-padded JPEG per frame, `settings.json` (the verified settings), `coordinator.log`, and `movie.mp4` if `GenerateMovie` is true and `ffmpeg` is on PATH (otherwise a warning is logged; `Verify` deliberately leaves `GenerateMovie` alone and the coordinator keeps the result in its runtime `makeMovie` field).

An unfinished run (stopped, crashed, or finished without ffmpeg) is continued with `-mode=coordinator -resume=<run folder>`; workers start as usual. `ResumeCoordinator` loads the run's saved settings with `loadRunSettings` (`settings.json`, or the single `.json` in older run folders; `SavePath`/`RunName` come from the folder path, and `-settings` is ignored). It counts a frame as done only if its JPEG fully decodes at the run's size (`completedFrames`), then `resumeFrom` drops those frames from `taskCount` and `generateTasks` skips them while keeping frame numbering unchanged. The log is appended to. If no frames are missing it goes straight to making the movie. Frames are saved through `saveFrame` (write `NNN.jpg.tmp`, then rename) so an existing frame file is always complete; resume works at whole-frame granularity, so frames that were partly assembled in memory are rendered again.

## Architecture

Two roles in one binary, selected in `main.go`. All communication is bidirectional net/rpc over TCP via the external `github.com/BrugadaSyndrome/multirpc` module; logging uses `github.com/BrugadaSyndrome/bslogger`. Both are the author's own modules (split out of this repo), so API changes there show up here as dependency bumps.

**Coordinator** (`coordinator/`) is an RPC server. On startup it:
1. `planTasks` computes the frame count for each `TransitionSettings` entry with `transitionSettings.frameCount()`, splits a frame into task rectangles with `task.Regions(TaskGeneration, width, height)` (stored in `regions`), and sets `taskCount = len(regions) × imageCount`. The coordinator shuts down when `taskIngestedCount == taskCount`; because generation and the count both come from `regions`, they cannot drift apart (`TestPlannedTasksMatchGeneratedTasks` guards this).
2. `generateTasks` goroutine walks each transition frame by frame, getting each frame's center and magnification from `transitionSettings.frame(i)`, and pushes one `task.Task` per region into the buffered `tasksTodo` channel. `tasksTodo` is never closed, because tasks from departed workers are pushed back into it. Frame `i` is at `start × step^i` (or `÷` when zooming out); the first frame is exactly the start values and the last frame reaches the end magnification (exactly, or less than one step past) and exactly the end center. Consecutive transitions that share an endpoint therefore render that boundary frame twice.
3. `ingestTasks` goroutine reads finished tasks from `tasksDone` and passes each to `ingestTask`, which copies the task's `Pixels` row by row into a per-frame `image.RGBA` (`images` map keyed by frame number), and writes the JPEG and frees memory when that frame's `PixelsLeft` reaches 0.

Coordinator data ownership (see the comment on the `Coordinator` struct): `clients` and `tasksHandedOut` are guarded by `mutex`; `images` belongs to the ingest goroutine alone and is never locked; progress counters (`taskGeneratedCount`, `taskIngestedCount`, `imageCompletedCount`, `imagesInProgress`) are `sync/atomic` types with one writer each, read by `status()` for the heartbeat. Fields set before `start()` (settings, sizes, `regions`, `taskCount`, `completed`, `makeMovie`, `logger`) are read-only afterwards. `NewCoordinator` and `ResumeCoordinator` share `newCoordinator` (builds the struct) and `start` (ffmpeg check, RPC server, goroutines); the server must start last. New shared state should pick one of these patterns explicitly.

RPC methods exposed to workers: `RegisterWorker`, `DeRegisterWorker`, `GetTask`, `ReturnTask`, `GetMandelbrotSettings`, `RollCall`. The end of work is "every task ingested", not "every task handed out":
- `GetTask` blocks until a task is queued or `allTasksDone` is closed (by `ingestTasks` once `taskIngestedCount == taskCount`), then returns `task.NoMoreTasksMessage`. RPC errors arrive as strings, so the worker compares against that shared constant.
- The coordinator tracks tasks per worker in `tasksHandedOut`. On deregistration (or a missed roll call, checked every minute) unreturned tasks are requeued in a background goroutine (`requeue`), so callers never block on a full channel or while holding the mutex.
- `ReturnTask` only accepts a task still recorded as handed out to the returning worker and removes it from `tasksHandedOut`; late copies from removed workers and repeat returns are dropped, which keeps the exact-count shutdown correct. It takes only `Pixels` from the worker (bounds and image number come from the coordinator's record) and rejects a result whose size is not `task.ResultSize()` with an error, leaving the task assigned so it is requeued when the worker leaves.
- `GetTask`/`DeRegisterWorker` return errors for unknown workers (a worker dropped by roll call may still call them). A worker stops itself on any `GetTask` error rather than exiting the whole process.
- `RegisterWorker` takes a `task.Registration` and rejects a `ProtocolVersion` different from `task.ProtocolVersion` (gob silently ignores unknown fields, so mismatched builds would otherwise produce blank images). **Bump `task.ProtocolVersion` whenever an RPC payload changes shape.** It also rejects duplicate addresses and, once `allTasksDone` is closed, returns `task.NoMoreTasksMessage` (the worker then stops quietly). `workerWait.Add` happens under the mutex and `allTasksDone` is closed under the mutex, so every `Add` precedes both its `Done` and the final `workerWait.Wait()`. Keep this ordering if you touch registration.

Worker-facing coordinator methods are tested in `coordinator_test.go` by building a `Coordinator` struct directly (`newTestCoordinator`/`addTestWorker`) without starting servers or goroutines. `ingest_test.go`'s `newIngestTestCoordinator` adds a temporary run folder for saving frames; `resume_test.go` covers resume planning and `saveFrame`.

**Worker** (`worker/`) is both an RPC server (for the coordinator's `Worker.RollCall`) and a client. It registers, fetches the `mandelbrot.Settings` from the coordinator (so render settings live only in the coordinator's JSON), then loops `GetTask` → `render` (fills `Pixels` for every pixel of `Bounds`) → `ReturnTask` until told there is no more work.

**Shared packages:**
- `task/` — the RPC payloads. A `Task` is a rectangle (`Bounds`) of one image plus that image's `Frame` (center and magnification); the coordinator sends it without pixel data (~300 bytes) and the worker returns it with `Pixels` as packed RGBA, row by row within `Bounds` — the same layout as `image.RGBA.Pix`, so ingesting is a row copy. `Generation` enum (`Row`, `Column`, `Image`, `Grid`, set by the integer `TaskGeneration` in the coordinator JSON) only selects how `Regions` splits a frame. `Registration` carries the worker address and protocol version.
- `mandelbrot/` — pure computation: pixel→complex mapping from a `task.Frame` plus column/row (scaled by the image's shorter side), escape-time with periodicity check, smooth coloring, grid super-sampling, palette generation from `GeneratePaletteSettings`.
- `misc/` — helpers: `CheckError(err, logger, severity)` is the pervasive error-handling idiom (Fatal exits), plus file IO, lerp/easing, free-port and local-IP discovery, and the empty `Nothing` type used for RPC args/replies.

## Conventions

- Settings structs are loaded from JSON and then normalized by a `Verify()` method that replaces missing/out-of-range values with defaults rather than erroring. Add new settings the same way (exported field + default in `Verify`).
- Exported struct fields are what crosses RPC (gob encoding); unexported fields like `logger` are not sent.
- When no address is configured, both sides default to the first non-loopback IPv4 on port 51000.

## Gotchas

- `.gitignore`'s `run_*` pattern matches the tracked `run_*.cmd` scripts, so a new `run_*` file needs `git add -f`. Ignore the Linux build output as `/DistributedMandelbrot` (anchored); an unanchored `mandelbrot` pattern would also ignore the `mandelbrot/` package.
- Changing anything that affects pixel output (rendering, settings defaults, JPEG encoding) makes a resumed run mix old and new frames. The saved settings protect against settings changes, not code changes.
- Grid generation splits each frame into `task.GridDivisions`² cells (the last row/column of cells absorbs the remainder; empty cells in frames smaller than the grid are skipped).
- `NewCoordinator` and `NewWorker` return pointers because the RPC server and background goroutines must share one instance (they hold a `sync.Mutex` / `sync.Once`).
- multirpc's `TcpServer.Stop()` panics if called twice (it closes a channel and calls `WaitGroup.Done`). The worker funnels every stop through `Worker.shutdown()` (guarded by `sync.Once`), which also closes `stopped` so the tickers goroutine exits. Never call `Server.Stop()` directly in the worker. The worker's `tasksCompleted` counter is atomic (written by `processTasks`, read by `status()` in the heartbeat), matching the coordinator's pattern.
