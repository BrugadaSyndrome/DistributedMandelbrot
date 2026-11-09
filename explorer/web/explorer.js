// Mandelbrot explorer page. The server computes tiles of smooth iteration counts; this page colors them, so palette
// changes never need the server to compute anything. See explorer/explorer.go for the API.

// How long the view must stay still before tiles are requested
const RENDER_DELAY_MS = 400;
// How often the active tab tells the server it is still open, and how often a turned away tab tries again
const HEARTBEAT_MS = 10000;
// The server's cap on tiles per render request (maxTilesPerRender)
const MAX_TILES_PER_RENDER = 1024;
// Size of the header before each tile in a render stream (frameHeaderSize)
const FRAME_HEADER_SIZE = 28;
const FRAME_CACHED = 1;
// Coarser levels drawn under missing tiles while they load, and finer ones kept when zooming out
const PLACEHOLDER_LEVELS_COARSER = 8;
const PLACEHOLDER_LEVELS_FINER = 2;
// The view's center stays within the set's radius
const CENTER_LIMIT = 2;
// Tiles kept in the browser, by memory: samples plus the colored canvas
const TILE_MEMORY_BUDGET = 768 * 1024 * 1024;
// Time spent recoloring tiles per animation frame after a color change, so the page stays responsive
const RECOLOR_BUDGET_MS = 25;

const canvas = document.getElementById("view");
const context = canvas.getContext("2d");
const $ = (id) => document.getElementById(id);

const state = {
    config: null,
    settings: null,      // normalized by the server, including the generated Palette
    session: null,
    view: {x: 0, y: 0, zoom: 0},
    tiles: new Map(),    // "level/x/y" -> tile, all computed with computeKey()
    tileBytes: 0,
    colorVersion: 0,     // bumped when colors change; tiles colored with an older version are recolored lazily
    colors: null,        // palette lookup tables for colorVersion
    frame: 0,            // draw counter, used to see which tiles were drawn most recently
    drawPending: false,
    renderTimer: 0,
    render: null,        // {controller, requested, received, cached}
    status: null,        // the last heartbeat's answer
    claimTimer: 0,
    heartbeatTimer: 0,
    error: "",
    focusGradientHandle: null, // a palette row moved with the keyboard, whose handle keeps focus after the form is rebuilt
};

// ---------------------------------------------------------------------------------------------------------------------
// Geometry. Everything on the canvas is in device pixels; complex y grows downward like the movie's pixel mapping.

function pixelSize(zoom) {
    return state.config.LevelZeroSpan / state.config.TileSize * Math.pow(2, -zoom);
}

function tileSpan(level) {
    return state.config.LevelZeroSpan * Math.pow(2, -level);
}

function renderLevel() {
    const {MinZoomLevel, MaxZoomLevel} = state.config.Limits;
    return Math.min(Math.max(Math.ceil(state.view.zoom - 1e-9), MinZoomLevel), MaxZoomLevel);
}

function toComplex(sx, sy) {
    const size = pixelSize(state.view.zoom);
    return {x: state.view.x + (sx - canvas.width / 2) * size, y: state.view.y + (sy - canvas.height / 2) * size};
}

// The tiles of a level that overlap the view and the plane, nearest to the center first
function visibleTiles(level) {
    const size = pixelSize(state.view.zoom);
    const span = tileSpan(level);
    const limit = state.config.PlaneLimit;
    const left = state.view.x - canvas.width / 2 * size;
    const top = state.view.y - canvas.height / 2 * size;
    const right = state.view.x + canvas.width / 2 * size;
    const bottom = state.view.y + canvas.height / 2 * size;
    const x0 = Math.floor(Math.max(left, -limit) / span), x1 = Math.ceil(Math.min(right, limit) / span) - 1;
    const y0 = Math.floor(Math.max(top, -limit) / span), y1 = Math.ceil(Math.min(bottom, limit) / span) - 1;
    const keys = [];
    for (let y = y0; y <= y1; y++) {
        for (let x = x0; x <= x1; x++) {
            const dx = (x + 0.5) * span - state.view.x, dy = (y + 0.5) * span - state.view.y;
            keys.push({level, x, y, distance: dx * dx + dy * dy});
        }
    }
    return keys.sort((a, b) => a.distance - b.distance);
}

function tileKey(level, x, y) {
    return `${level}/${x}/${y}`;
}

// ---------------------------------------------------------------------------------------------------------------------
// Coloring: a port of the mandelbrot package's getPaletteColor, getSmoothColor and GetColorMultiple.

function buildColors() {
    const {Palette, EscapeColor, SmoothColoring, MaxIterations} = state.settings;
    const n = Palette.length;
    const r = new Uint8Array(n), g = new Uint8Array(n), b = new Uint8Array(n);
    Palette.forEach((c, i) => { r[i] = c.R; g[i] = c.G; b[i] = c.B; });
    state.colors = {r, g, b, n, escape: EscapeColor, smooth: SmoothColoring, max: MaxIterations};
}

function colorTile(tile) {
    const {r, g, b, n, escape, smooth, max} = state.colors;
    const samples = tile.samples;
    const perPixel = tile.superSampling * tile.superSampling;
    const image = tile.context.createImageData(tile.canvas.width, tile.canvas.height);
    const data = image.data;
    // The color of a sample's palette entry; Go treats a whole iteration count of MaxIterations as inside the set.
    // Unlike Go's uint conversion, the modulo stays non-negative for negative values (points that escape at once).
    const channel = (v, k) => {
        const u = Math.floor(v);
        if (u >= max) return k === 0 ? escape.R : k === 1 ? escape.G : escape.B;
        const i = ((u % n) + n) % n;
        return k === 0 ? r[i] : k === 1 ? g[i] : b[i];
    };
    for (let p = 0, s = 0; p < data.length; p += 4) {
        let red = 0, green = 0, blue = 0;
        for (let k = 0; k < perPixel; k++, s++) {
            const v = samples[s];
            if (smooth) {
                const fraction = v - Math.floor(v);
                red += (channel(v, 0) + (channel(v + 1, 0) - channel(v, 0)) * fraction) | 0;
                green += (channel(v, 1) + (channel(v + 1, 1) - channel(v, 1)) * fraction) | 0;
                blue += (channel(v, 2) + (channel(v + 1, 2) - channel(v, 2)) * fraction) | 0;
            } else {
                red += channel(v, 0);
                green += channel(v, 1);
                blue += channel(v, 2);
            }
        }
        data[p] = (red / perPixel) | 0;
        data[p + 1] = (green / perPixel) | 0;
        data[p + 2] = (blue / perPixel) | 0;
        data[p + 3] = 255;
    }
    tile.context.putImageData(image, 0, 0);
    tile.colorVersion = state.colorVersion;
}

// ---------------------------------------------------------------------------------------------------------------------
// Drawing

function requestDraw() {
    if (!state.drawPending) {
        state.drawPending = true;
        requestAnimationFrame(draw);
    }
}

function resizeCanvas() {
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.round(canvas.clientWidth * ratio);
    canvas.height = Math.round(canvas.clientHeight * ratio);
}

function draw() {
    state.drawPending = false;
    if (!state.config) return;
    state.frame++;
    context.fillStyle = "#05070d";
    context.fillRect(0, 0, canvas.width, canvas.height);

    const level = renderLevel();
    const {MinZoomLevel, MaxZoomLevel} = state.config.Limits;
    // Coarse placeholders first, then finer tiles left from zooming out, then the level being rendered on top
    const levels = [];
    for (let l = Math.max(level - PLACEHOLDER_LEVELS_COARSER, MinZoomLevel); l < level; l++) levels.push(l);
    for (let l = Math.min(level + PLACEHOLDER_LEVELS_FINER, MaxZoomLevel); l > level; l--) levels.push(l);
    levels.push(level);

    const deadline = performance.now() + RECOLOR_BUDGET_MS;
    let recolorLeft = false;
    const size = pixelSize(state.view.zoom);
    context.imageSmoothingEnabled = true;
    for (const l of levels) {
        const span = tileSpan(l);
        const drawnSize = span / size;
        for (const {x, y} of visibleTiles(l)) {
            const tile = state.tiles.get(tileKey(l, x, y));
            if (!tile) continue;
            if (tile.colorVersion !== state.colorVersion) {
                if (performance.now() < deadline || tile.colorVersion < 0) {
                    colorTile(tile);
                } else {
                    recolorLeft = true;
                }
            }
            tile.lastDrawn = state.frame;
            // Snap edges to whole pixels so neighboring tiles meet without gaps
            const left = Math.floor((x * span - state.view.x) / size + canvas.width / 2);
            const top = Math.floor((y * span - state.view.y) / size + canvas.height / 2);
            const right = Math.floor(((x + 1) * span - state.view.x) / size + canvas.width / 2);
            const bottom = Math.floor(((y + 1) * span - state.view.y) / size + canvas.height / 2);
            if (drawnSize >= 0.5) {
                context.drawImage(tile.canvas, left, top, right - left, bottom - top);
            }
        }
    }
    if (recolorLeft) requestDraw();
    updateLocation();
    updateStatus();
}

// ---------------------------------------------------------------------------------------------------------------------
// Tiles

function computeKey(s = state.settings) {
    return `${s.MaxIterations}|${s.Boundary}|${s.SuperSampling}`;
}

function clearTiles() {
    state.tiles.clear();
    state.tileBytes = 0;
}

function addTile(level, x, y, samples, superSampling) {
    const tileCanvas = document.createElement("canvas");
    tileCanvas.width = tileCanvas.height = state.config.TileSize;
    const tile = {
        level, x, y, samples, superSampling,
        canvas: tileCanvas,
        context: tileCanvas.getContext("2d"),
        colorVersion: -1,
        lastDrawn: state.frame,
        bytes: samples.byteLength + tileCanvas.width * tileCanvas.height * 4,
    };
    const key = tileKey(level, x, y);
    const old = state.tiles.get(key);
    if (old) state.tileBytes -= old.bytes;
    state.tiles.set(key, tile);
    state.tileBytes += tile.bytes;
    evictTiles();
    requestDraw();
}

// Drops the tiles drawn least recently once the budget is used up, never the ones on screen now
function evictTiles() {
    if (state.tileBytes <= TILE_MEMORY_BUDGET) return;
    const candidates = [...state.tiles.entries()]
        .filter(([, tile]) => tile.lastDrawn < state.frame)
        .sort((a, b) => a[1].lastDrawn - b[1].lastDrawn);
    for (const [key, tile] of candidates) {
        if (state.tileBytes <= TILE_MEMORY_BUDGET * 0.8) break;
        state.tiles.delete(key);
        state.tileBytes -= tile.bytes;
    }
}

async function decompress(bytes) {
    const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream("deflate-raw"));
    return new Float32Array(await new Response(stream).arrayBuffer());
}

// ---------------------------------------------------------------------------------------------------------------------
// Rendering: after the view has been still for RENDER_DELAY_MS, ask the server for the visible tiles this page does
// not have yet. Starting a new request aborts the old one, which also drops its queued tiles on the server.

function scheduleRender(delay = RENDER_DELAY_MS) {
    clearTimeout(state.renderTimer);
    state.renderTimer = setTimeout(render, delay);
}

function stopRender() {
    clearTimeout(state.renderTimer);
    if (state.render) state.render.controller.abort();
    state.render = null;
}

async function render() {
    if (!state.session || !state.settings) return;
    stopRender();

    const level = renderLevel();
    const visible = visibleTiles(level);
    const wanted = visible
        .filter(({x, y}) => !state.tiles.has(tileKey(level, x, y)))
        .slice(0, MAX_TILES_PER_RENDER);
    if (wanted.length === 0) {
        state.render = {controller: new AbortController(), requested: 0, visible: visible.length};
        updateStatus();
        return;
    }

    const settings = state.settings;
    const key = computeKey(settings);
    const job = {controller: new AbortController(), requested: wanted.length, received: 0, cached: 0, started: performance.now()};
    state.render = job;
    updateStatus();

    try {
        const response = await fetch("/api/render", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({
                Session: state.session,
                Boundary: settings.Boundary,
                MaxIterations: settings.MaxIterations,
                SuperSampling: settings.SuperSampling,
                Tiles: wanted.map(({level, x, y}) => ({Level: level, X: x, Y: y})),
            }),
            signal: job.controller.signal,
        });
        if (response.status === 409) {
            lostSession();
            return;
        }
        if (!response.ok) {
            showError(`Render failed: ${(await response.json()).Error}`);
            return;
        }

        const reader = response.body.getReader();
        let buffer = new Uint8Array(0);
        const pending = [];
        for (;;) {
            const {done, value} = await reader.read();
            if (done) break;
            buffer = concat(buffer, value);
            let offset = 0;
            while (buffer.length - offset >= FRAME_HEADER_SIZE) {
                const header = new DataView(buffer.buffer, buffer.byteOffset + offset, FRAME_HEADER_SIZE);
                const length = header.getUint32(24, true);
                if (buffer.length - offset - FRAME_HEADER_SIZE < length) break;
                const tileLevel = header.getInt32(0, true);
                const cached = (header.getUint32(4, true) & FRAME_CACHED) !== 0;
                const x = Number(header.getBigInt64(8, true));
                const y = Number(header.getBigInt64(16, true));
                const payload = buffer.slice(offset + FRAME_HEADER_SIZE, offset + FRAME_HEADER_SIZE + length);
                offset += FRAME_HEADER_SIZE + length;
                pending.push(decompress(payload).then((samples) => {
                    // Settings may have changed while this tile was on its way
                    if (computeKey() !== key) return;
                    addTile(tileLevel, x, y, samples, settings.SuperSampling);
                    job.received++;
                    if (cached) job.cached++;
                    updateStatus();
                }));
            }
            buffer = buffer.slice(offset);
        }
        await Promise.all(pending);
        job.finished = performance.now();
        updateStatus();
    } catch (error) {
        if (error.name !== "AbortError") showError(`Render failed: ${error.message}`);
    }
}

function concat(a, b) {
    if (a.length === 0) return b;
    const joined = new Uint8Array(a.length + b.length);
    joined.set(a);
    joined.set(b, a.length);
    return joined;
}

// ---------------------------------------------------------------------------------------------------------------------
// View changes

function setView(x, y, zoom) {
    const {MinZoomLevel, MaxZoomLevel} = state.config.Limits;
    state.view.zoom = Math.min(Math.max(zoom, MinZoomLevel), MaxZoomLevel);
    state.view.x = Math.min(Math.max(x, -CENTER_LIMIT), CENTER_LIMIT);
    state.view.y = Math.min(Math.max(y, -CENTER_LIMIT), CENTER_LIMIT);
    requestDraw();
    scheduleRender();
    saveViewToHash();
}

// Zooms by delta levels keeping the point under (sx, sy) in place
function zoomAt(sx, sy, delta) {
    const before = toComplex(sx, sy);
    const {MinZoomLevel, MaxZoomLevel} = state.config.Limits;
    const zoom = Math.min(Math.max(state.view.zoom + delta, MinZoomLevel), MaxZoomLevel);
    const size = pixelSize(zoom);
    setView(before.x - (sx - canvas.width / 2) * size, before.y - (sy - canvas.height / 2) * size, zoom);
}

function zoomCenter(delta) {
    zoomAt(canvas.width / 2, canvas.height / 2, delta);
}

// Pans by a number of device pixels
function panBy(dx, dy) {
    const size = pixelSize(state.view.zoom);
    setView(state.view.x + dx * size, state.view.y + dy * size, state.view.zoom);
}

let hashTimer = 0;

function saveViewToHash() {
    clearTimeout(hashTimer);
    hashTimer = setTimeout(() => {
        const {x, y, zoom} = state.view;
        history.replaceState(null, "", `#${x.toPrecision(17)},${y.toPrecision(17)},${zoom.toFixed(3)}`);
    }, RENDER_DELAY_MS);
}

function viewFromHash() {
    const parts = location.hash.slice(1).split(",").map(Number);
    if (parts.length === 3 && parts.every(Number.isFinite)) {
        return {x: parts[0], y: parts[1], zoom: parts[2]};
    }
    return null;
}

function setupInput() {
    canvas.addEventListener("wheel", (event) => {
        event.preventDefault();
        const ratio = window.devicePixelRatio || 1;
        const perUnit = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? 0.05 : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? 1 : 0.0015;
        zoomAt(event.offsetX * ratio, event.offsetY * ratio, -event.deltaY * perUnit);
    }, {passive: false});

    let drag = null;
    canvas.addEventListener("pointerdown", (event) => {
        if (event.button !== 0) return;
        canvas.setPointerCapture(event.pointerId);
        canvas.classList.add("dragging");
        canvas.focus();
        drag = {x: event.clientX, y: event.clientY};
    });
    canvas.addEventListener("pointermove", (event) => {
        if (!drag) return;
        const ratio = window.devicePixelRatio || 1;
        panBy(-(event.clientX - drag.x) * ratio, -(event.clientY - drag.y) * ratio);
        drag = {x: event.clientX, y: event.clientY};
    });
    const endDrag = () => {
        drag = null;
        canvas.classList.remove("dragging");
    };
    canvas.addEventListener("pointerup", endDrag);
    canvas.addEventListener("pointercancel", endDrag);

    // The browser's own "Save image as" cannot be given a file name, so right click opens this page's menu instead
    const menu = $("context-menu");
    canvas.addEventListener("contextmenu", (event) => {
        event.preventDefault();
        menu.hidden = false;
        menu.style.left = `${Math.min(event.clientX, window.innerWidth - menu.offsetWidth - 4)}px`;
        menu.style.top = `${Math.min(event.clientY, window.innerHeight - menu.offsetHeight - 4)}px`;
        $("save-image").focus();
    });
    const closeMenu = () => { menu.hidden = true; };
    window.addEventListener("pointerdown", (event) => {
        if (!menu.contains(event.target)) closeMenu();
    });
    window.addEventListener("keydown", (event) => {
        if (event.key === "Escape") closeMenu();
    });
    canvas.addEventListener("wheel", closeMenu);
    $("save-image").addEventListener("click", () => {
        closeMenu();
        saveImage();
    });

    window.addEventListener("keydown", (event) => {
        if (event.target.closest("input, select, textarea") || event.ctrlKey || event.metaKey || event.altKey) return;
        const stepX = canvas.width / 8, stepY = canvas.height / 8;
        const moves = {
            ArrowLeft: [-stepX, 0], a: [-stepX, 0], A: [-stepX, 0],
            ArrowRight: [stepX, 0], d: [stepX, 0], D: [stepX, 0],
            ArrowUp: [0, -stepY], w: [0, -stepY], W: [0, -stepY],
            ArrowDown: [0, stepY], s: [0, stepY], S: [0, stepY],
        };
        if (moves[event.key]) {
            panBy(...moves[event.key]);
        } else if (event.key === "+" || event.key === "=") {
            zoomCenter(0.5);
        } else if (event.key === "-" || event.key === "_") {
            zoomCenter(-0.5);
        } else {
            return;
        }
        event.preventDefault();
    });

    $("zoom-in").addEventListener("click", () => zoomCenter(0.5));
    $("zoom-out").addEventListener("click", () => zoomCenter(-0.5));
    $("reset-view").addEventListener("click", () => setView(state.config.StartX, state.config.StartY, state.config.StartZoom));
    $("toggle-sidebar").addEventListener("click", () => {
        const hidden = document.body.classList.toggle("sidebar-hidden");
        $("toggle-sidebar").setAttribute("aria-expanded", String(!hidden));
        try {
            localStorage.setItem("explorer.sidebarHidden", hidden ? "1" : "");
        } catch {
            // storage may be unavailable; the sidebar just starts open next time
        }
    });

    new ResizeObserver(() => {
        resizeCanvas();
        requestDraw();
        scheduleRender();
    }).observe(canvas);
}

// ---------------------------------------------------------------------------------------------------------------------
// Settings form. Every change goes through /api/settings, which applies the server's defaults, limits and palette
// generation, so the page and the movie mode share one implementation of each.

function hex(c) {
    return "#" + [c.R, c.G, c.B].map((v) => v.toString(16).padStart(2, "0")).join("");
}

function rgba(value) {
    return {R: parseInt(value.slice(1, 3), 16), G: parseInt(value.slice(3, 5), 16), B: parseInt(value.slice(5, 7), 16), A: 255};
}

function addGradientRow(gradient) {
    const row = $("gradient-row").content.firstElementChild.cloneNode(true);
    row.querySelector(".start").value = hex(gradient.StartColor);
    row.querySelector(".end").value = hex(gradient.EndColor);
    row.querySelector(".count").value = gradient.NumberColors;
    row.querySelector(".remove").addEventListener("click", () => {
        row.remove();
        settingsChanged();
    });
    setupReordering(row);
    $("gradients").append(row);
}

// setupReordering lets a gradient row be moved by dragging its handle, or with the arrow keys while the handle has
// focus. The palette is generated from the gradients in order, so a move is sent like any other setting change.
function setupReordering(row) {
    const handle = row.querySelector(".handle");
    const position = () => [...row.parentElement.children].indexOf(row);

    handle.addEventListener("keydown", (event) => {
        if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
        // Keep the arrow keys from also panning the view
        event.preventDefault();
        event.stopPropagation();
        const neighbor = event.key === "ArrowUp" ? row.previousElementSibling : row.nextElementSibling;
        if (!neighbor) return;
        if (event.key === "ArrowUp") neighbor.before(row); else neighbor.after(row);
        handle.focus();
        // The form is rebuilt from the server's answer, so remember which handle to give focus back to
        state.focusGradientHandle = position();
        settingsChanged(0);
    });

    handle.addEventListener("pointerdown", (event) => {
        if (event.button !== 0) return;
        event.preventDefault();
        row.classList.add("dragging");
        const startPosition = position();

        // Follow the pointer on the window rather than capturing it on the handle: moving the row within the page
        // releases any capture, which would leave the drag stuck
        const move = (moveEvent) => {
            if (!row.isConnected) return;
            // Put the row before or after whichever row the pointer is over, depending on which half it is in
            for (const other of row.parentElement.children) {
                const rect = other.getBoundingClientRect();
                if (other === row || moveEvent.clientY < rect.top || moveEvent.clientY >= rect.bottom) continue;
                if (moveEvent.clientY < rect.top + rect.height / 2) other.before(row); else other.after(row);
                break;
            }
        };
        const end = () => {
            window.removeEventListener("pointermove", move);
            window.removeEventListener("pointerup", end);
            window.removeEventListener("pointercancel", end);
            row.classList.remove("dragging");
            if (row.isConnected && position() !== startPosition) settingsChanged(0);
        };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", end);
        window.addEventListener("pointercancel", end);
    });
}

function fillForm(s) {
    $("max-iterations").value = s.MaxIterations;
    $("boundary").value = s.Boundary;
    $("super-sampling").value = s.SuperSampling;
    $("smooth-coloring").checked = s.SmoothColoring;
    $("escape-color").value = hex(s.EscapeColor);
    $("gradients").replaceChildren();
    for (const gradient of s.GeneratePaletteSettings || []) addGradientRow(gradient);
    if (state.focusGradientHandle !== null) {
        $("gradients").children[state.focusGradientHandle]?.querySelector(".handle").focus();
        state.focusGradientHandle = null;
    }
    drawPalettePreview(s.Palette);
}

function readForm() {
    return {
        ...state.settings,
        MaxIterations: Number($("max-iterations").value),
        Boundary: Number($("boundary").value),
        SuperSampling: Number($("super-sampling").value),
        SmoothColoring: $("smooth-coloring").checked,
        EscapeColor: rgba($("escape-color").value),
        GeneratePaletteSettings: [...$("gradients").children].map((row) => ({
            StartColor: rgba(row.querySelector(".start").value),
            EndColor: rgba(row.querySelector(".end").value),
            NumberColors: Number(row.querySelector(".count").value),
        })),
        Palette: null,
    };
}

function drawPalettePreview(palette) {
    const preview = $("palette-preview");
    preview.width = Math.max(palette.length, 1);
    const previewContext = preview.getContext("2d");
    palette.forEach((c, i) => {
        previewContext.fillStyle = hex(c);
        previewContext.fillRect(i, 0, 1, 1);
    });
}

let settingsTimer = 0;

function settingsChanged(delay = 150) {
    clearTimeout(settingsTimer);
    settingsTimer = setTimeout(() => submitSettings(readForm()), delay);
}

async function submitSettings(form) {
    if (!state.session) return;
    const response = await fetch("/api/settings", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({Session: state.session, Settings: form}),
    });
    if (response.status === 409) {
        lostSession();
        return;
    }
    if (!response.ok) {
        showError(`Settings were not accepted: ${(await response.json()).Error}`);
        return;
    }
    applySettings(await response.json());
}

function applySettings(normalized) {
    const recompute = !state.settings || computeKey(normalized) !== computeKey(state.settings);
    state.settings = normalized;
    fillForm(normalized);
    buildColors();
    state.colorVersion++;
    if (recompute) {
        stopRender();
        clearTiles();
        scheduleRender(0);
    }
    requestDraw();
}

function setupForm() {
    const {Limits} = state.config;
    $("max-iterations").max = Limits.MaxIterations;
    const superSampling = $("super-sampling");
    for (let i = 1; i <= Limits.MaxSuperSampling; i++) {
        superSampling.add(new Option(i === 1 ? "Off (1×1)" : `${i}×${i}`, String(i)));
    }
    $("limits").replaceChildren(...[
        ["Max iterations", Limits.MaxIterations.toLocaleString()],
        ["Super sampling", `${Limits.MaxSuperSampling}×${Limits.MaxSuperSampling}`],
        ["Zoom levels", `${Limits.MinZoomLevel} to ${Limits.MaxZoomLevel}`],
    ].flatMap(([term, value]) => {
        const dt = document.createElement("dt");
        const dd = document.createElement("dd");
        dt.textContent = term;
        dd.textContent = value;
        return [dt, dd];
    }));

    // Everything applies when committed: numbers on Enter or leaving the field, colors when the picker closes.
    // Applying a color while its picker is open would rebuild the form and close the picker after one change.
    for (const id of ["max-iterations", "boundary", "super-sampling", "smooth-coloring", "escape-color"]) {
        $(id).addEventListener("change", () => settingsChanged(0));
    }
    $("gradients").addEventListener("change", () => settingsChanged(0));
    $("add-gradient").addEventListener("click", () => {
        const gradients = state.settings.GeneratePaletteSettings || [];
        const last = gradients[gradients.length - 1];
        addGradientRow({
            StartColor: last ? last.EndColor : {R: 0, G: 0, B: 0, A: 255},
            EndColor: {R: 255, G: 255, B: 255, A: 255},
            NumberColors: 25,
        });
        settingsChanged(0);
    });
    $("reset-settings").addEventListener("click", () => submitSettings(state.config.Defaults));
    $("copy-location").addEventListener("click", copyLocation);
}

// ---------------------------------------------------------------------------------------------------------------------
// Location and status

// The movie's Magnification makes the shorter side of the frame 1/Magnification units tall
function magnification() {
    return 1 / (pixelSize(state.view.zoom) * Math.min(canvas.width, canvas.height));
}

function updateLocation() {
    $("center-x").textContent = state.view.x.toPrecision(17);
    $("center-y").textContent = state.view.y.toPrecision(17);
    $("magnification").textContent = magnification().toPrecision(6);
    $("zoom-level").textContent = `${state.view.zoom.toFixed(2)} (tiles at ${renderLevel()})`;
}

async function copyLocation() {
    const text = `"EndX": ${state.view.x.toPrecision(17)},\n"EndY": ${state.view.y.toPrecision(17)},\n"MagnificationEnd": ${magnification().toPrecision(6)}`;
    try {
        await navigator.clipboard.writeText(text);
        $("copy-location").textContent = "Copied";
    } catch {
        $("copy-location").textContent = "Copy failed";
    }
    setTimeout(() => { $("copy-location").textContent = "Copy for a movie"; }, 1500);
}

// imageFileName names a saved view after where it is: center x, center y, magnification and zoom level
function imageFileName() {
    const {x, y, zoom} = state.view;
    return `${x.toPrecision(17)}_${y.toPrecision(17)}_${magnification().toPrecision(6)}_${zoom.toFixed(2)}.png`;
}

// saveImage downloads the view as shown, at full device resolution and without the page's controls
function saveImage() {
    const name = imageFileName();
    canvas.toBlob((blob) => {
        if (!blob) {
            showError("Unable to save the image");
            return;
        }
        const link = document.createElement("a");
        link.href = URL.createObjectURL(blob);
        link.download = name;
        link.click();
        setTimeout(() => URL.revokeObjectURL(link.href), 10000);
    }, "image/png");
}

function showError(message) {
    state.error = message;
    updateStatus();
}

function updateStatus() {
    const parts = [];
    const job = state.render;
    if (job && job.requested === 0) {
        parts.push(`All ${job.visible} visible tiles loaded`);
    } else if (job) {
        const seconds = (((job.finished ?? performance.now()) - job.started) / 1000).toFixed(1);
        parts.push(job.received < job.requested
            ? `Rendering ${job.received}/${job.requested} tiles (${job.cached} cached)`
            : `${job.requested} tiles in ${seconds} s (${job.cached} cached)`);
    }
    if (state.status) {
        parts.push(`${state.status.Workers} worker${state.status.Workers === 1 ? "" : "s"}`);
        parts.push(`cache ${(state.status.CacheBytes / 1048576).toFixed(0)} MB`);
    }
    const status = $("status");
    status.textContent = parts.join(" · ");
    if (state.error) {
        const error = document.createElement("span");
        error.className = "error";
        error.textContent = (parts.length ? " · " : "") + state.error;
        status.append(error);
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// Session: only one tab may use the explorer. The first one keeps it; others wait until it is released or expires.

async function claimSession() {
    clearTimeout(state.claimTimer);
    let response;
    try {
        response = await fetch("/api/session", {method: "POST"});
    } catch {
        state.claimTimer = setTimeout(claimSession, HEARTBEAT_MS);
        return;
    }
    if (response.ok) {
        state.session = (await response.json()).Session;
        $("busy-overlay").hidden = true;
        clearInterval(state.heartbeatTimer);
        state.heartbeatTimer = setInterval(heartbeat, HEARTBEAT_MS);
        heartbeat();
        scheduleRender(0);
    } else {
        $("busy-overlay").hidden = false;
        state.claimTimer = setTimeout(claimSession, HEARTBEAT_MS);
    }
}

async function heartbeat() {
    if (!state.session) return;
    try {
        const response = await fetch("/api/session/heartbeat", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({Session: state.session}),
        });
        if (response.status === 409) {
            lostSession();
            return;
        }
        if (response.ok) {
            state.status = await response.json();
            state.error = "";
            updateStatus();
        }
    } catch {
        showError("The explorer server is not responding");
    }
}

// The session expired (e.g. the computer slept): take it back if no other tab has it, or wait like a new tab
function lostSession() {
    state.session = null;
    clearInterval(state.heartbeatTimer);
    stopRender();
    claimSession();
}

function releaseSession() {
    if (state.session) {
        navigator.sendBeacon("/api/session/release", JSON.stringify({Session: state.session}));
        state.session = null;
        clearInterval(state.heartbeatTimer);
    }
}

// ---------------------------------------------------------------------------------------------------------------------

async function start() {
    try {
        if (localStorage.getItem("explorer.sidebarHidden")) {
            document.body.classList.add("sidebar-hidden");
            $("toggle-sidebar").setAttribute("aria-expanded", "false");
        }
    } catch {
        // storage may be unavailable
    }

    state.config = await (await fetch("/api/config")).json();
    resizeCanvas();
    const view = viewFromHash() ?? {x: state.config.StartX, y: state.config.StartY, zoom: state.config.StartZoom};
    state.view = {x: view.x, y: view.y, zoom: view.zoom};
    setupForm();
    setupInput();
    applySettings(state.config.Defaults);
    setView(view.x, view.y, view.zoom);

    window.addEventListener("pagehide", releaseSession);
    window.addEventListener("pageshow", (event) => {
        if (event.persisted) claimSession();
    });
    await claimSession();
    canvas.focus();
}

start().catch((error) => showError(`Unable to start: ${error.message}`));
