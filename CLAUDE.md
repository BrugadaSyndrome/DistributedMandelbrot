# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and run

Go module `DistributedMandelbrot` (Go 1.27, per `go.mod`). There are no tests or lint config in the repo.

```bash
go build            # produces DistributedMandelbrot(.exe)
go vet ./...
```

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
1. Computes the frame count for each `TransitionSettings` entry from the magnification start/end/step (log formula in `NewCoordinator`) and the total task count from `TaskGeneration`. The coordinator shuts down when `taskIngestedCount == taskCount`, so **any change to how tasks are generated must keep that count calculation in sync**.
2. `generateTasks` goroutine walks each transition frame by frame (exponential magnification, eased lerp of center X/Y) and pushes `task.Task`s into the buffered `tasksTodo` channel, closing it at the end.
3. `ingestTasks` goroutine reads finished tasks from `tasksDone`, paints pixels into a per-frame `image.RGBA` (`images` map keyed by frame number), and writes the JPEG and frees memory when that frame's `PixelsLeft` reaches 0.

RPC methods exposed to workers: `RegisterWorker`, `DeRegisterWorker`, `GetTask`, `ReturnTask`, `GetMandelbrotSettings`, `RollCall`. `GetTask` returns the error string `"all tasks handed out"` once `tasksTodo` is closed — the worker matches that exact string to exit cleanly. The coordinator tracks tasks per worker in `tasksHandedOut`; on deregistration (or a missed roll call, checked every minute) unreturned tasks are pushed back into `tasksTodo`.

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
