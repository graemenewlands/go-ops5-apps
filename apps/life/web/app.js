// ==============================================================================
// Conway's Game of Life — OPS5 WebAssembly Client Application
// ==============================================================================

(function () {
  'use strict';

  // State
  let isRunning = false;
  let animFrameId = null;
  let lastFrameTime = 0;
  let targetFps = 10;
  let gridWidth = 30;
  let gridHeight = 30;
  let cellSize = 20;
  let liveCellsSet = new Set(); // "r,c"
  let isMouseDown = false;
  let paintMode = true; // true = paint alive, false = erase
  let lastPaintedKey = null;

  // DOM Elements
  const loadingOverlay = document.getElementById('loading-overlay');
  const canvas = document.getElementById('life-canvas');
  const ctx = canvas.getContext('2d');
  const btnPlay = document.getElementById('btn-play');
  const playText = document.getElementById('play-text');
  const iconPlay = btnPlay.querySelector('.icon-play');
  const iconPause = btnPlay.querySelector('.icon-pause');
  const btnStep = document.getElementById('btn-step');
  const btnClear = document.getElementById('btn-clear');
  const patternSelect = document.getElementById('pattern-select');
  const gridSizeSelect = document.getElementById('grid-size-select');
  const speedSlider = document.getElementById('speed-slider');
  const speedLabel = document.getElementById('speed-label');
  const hoverCoord = document.getElementById('hover-coord');

  // Stats elements
  const statGen = document.getElementById('stat-generation');
  const statPop = document.getElementById('stat-population');
  const statCycles = document.getElementById('stat-cycles');
  const statLatency = document.getElementById('stat-latency');
  const statWmes = document.getElementById('stat-wmes');

  // Rule Inspector
  const btnToggleInspector = document.getElementById('btn-toggle-inspector');
  const btnCloseInspector = document.getElementById('btn-close-inspector');
  const inspectorDrawer = document.getElementById('inspector-drawer');
  const ruleSourceEditor = document.getElementById('rule-source-editor');
  const btnApplyRules = document.getElementById('btn-apply-rules');
  const btnResetRules = document.getElementById('btn-reset-rules');
  const rulesStatusMsg = document.getElementById('rules-status-msg');

  // ----------------------------------------------------------------------------
  // WebAssembly Loader
  // ----------------------------------------------------------------------------
  async function initWasm() {
    const go = new Go();
    try {
      let result;
      if (WebAssembly.instantiateStreaming) {
        result = await WebAssembly.instantiateStreaming(fetch('main.wasm'), go.importObject);
      } else {
        const resp = await fetch('main.wasm');
        const bytes = await resp.arrayBuffer();
        result = await WebAssembly.instantiate(bytes, go.importObject);
      }
      go.run(result.instance);

      // Initialize life engine once wasm functions are exposed
      waitForWasmReady();
    } catch (err) {
      console.error('Failed to load WebAssembly binary:', err);
      loadingOverlay.innerHTML = `
        <div style="color: #ef4444; font-size: 1.1rem; font-weight: bold;">Failed to load WebAssembly module</div>
        <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 8px;">${err.message}</div>
      `;
    }
  }

  function waitForWasmReady() {
    if (typeof window.lifeInit === 'function') {
      onWasmReady();
    } else {
      setTimeout(waitForWasmReady, 50);
    }
  }

  function onWasmReady() {
    console.log('OPS5 Game of Life WebAssembly module initialized successfully.');
    loadingOverlay.classList.add('hidden');

    // Initialize grid in Wasm
    initGrid(gridWidth, gridHeight);

    // Initial default pattern: Glider near top-left
    loadPresetPattern('glider');

    // Attach listeners
    setupEventListeners();
  }

  function initGrid(w, h) {
    gridWidth = w;
    gridHeight = h;
    const res = window.lifeInit(w, h);
    if (!res.success) {
      console.error('Failed to init life grid:', res.error);
      return;
    }
    resizeCanvas();
    updateStatsFromWasm(window.lifeGetState());
    drawGrid();
  }

  // ----------------------------------------------------------------------------
  // Canvas Rendering & Sizing
  // ----------------------------------------------------------------------------
  function resizeCanvas() {
    const maxWidth = Math.min(window.innerWidth - 80, 850);
    cellSize = Math.floor(maxWidth / gridWidth);
    if (cellSize < 8) cellSize = 8;
    if (cellSize > 30) cellSize = 30;

    const displayWidth = cellSize * gridWidth;
    const displayHeight = cellSize * gridHeight;
    const dpr = window.devicePixelRatio || 1;

    canvas.width = displayWidth * dpr;
    canvas.height = displayHeight * dpr;
    canvas.style.width = displayWidth + 'px';
    canvas.style.height = displayHeight + 'px';

    ctx.resetTransform?.();
    ctx.scale(dpr, dpr);
    drawGrid();
  }

  function drawGrid() {
    const totalWidth = cellSize * gridWidth;
    const totalHeight = cellSize * gridHeight;

    // Background
    ctx.fillStyle = '#0a0e17';
    ctx.fillRect(0, 0, totalWidth, totalHeight);

    // Grid lines
    ctx.strokeStyle = '#161f30';
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (let c = 0; c <= gridWidth; c++) {
      const x = c * cellSize;
      ctx.moveTo(x, 0);
      ctx.lineTo(x, totalHeight);
    }
    for (let r = 0; r <= gridHeight; r++) {
      const y = r * cellSize;
      ctx.moveTo(0, y);
      ctx.lineTo(totalWidth, y);
    }
    ctx.stroke();

    // Live cells
    ctx.fillStyle = '#00f2fe';
    ctx.shadowColor = 'rgba(0, 242, 254, 0.4)';
    ctx.shadowBlur = cellSize > 12 ? 8 : 0;

    const padding = Math.max(1, Math.floor(cellSize * 0.1));
    const cellDrawSize = cellSize - padding * 2;
    const radius = Math.min(4, Math.floor(cellDrawSize * 0.25));

    for (const key of liveCellsSet) {
      const [r, c] = key.split(',').map(Number);
      const x = c * cellSize + padding;
      const y = r * cellSize + padding;

      drawRoundedRect(ctx, x, y, cellDrawSize, cellDrawSize, radius);
    }

    ctx.shadowBlur = 0;
  }

  function drawRoundedRect(context, x, y, width, height, radius) {
    context.beginPath();
    context.moveTo(x + radius, y);
    context.lineTo(x + width - radius, y);
    context.quadraticCurveTo(x + width, y, x + width, y + radius);
    context.lineTo(x + width, y + height - radius);
    context.quadraticCurveTo(x + width, y + height, x + width - radius, y + height);
    context.lineTo(x + radius, y + height);
    context.quadraticCurveTo(x, y + height, x, y + height - radius);
    context.lineTo(x, y + radius);
    context.quadraticCurveTo(x, y, x + radius, y);
    context.closePath();
    context.fill();
  }

  // ----------------------------------------------------------------------------
  // Simulation Step & Animation Loop
  // ----------------------------------------------------------------------------
  function stepSimulation() {
    if (typeof window.lifeStep !== 'function') return;
    const stats = window.lifeStep();
    if (stats.success) {
      updateStatsFromWasm(stats);
      drawGrid();
    } else {
      console.error('OPS5 Life step error:', stats.error);
      stopSimulation();
    }
  }

  function updateStatsFromWasm(stats) {
    if (!stats || !stats.cells) return;

    liveCellsSet.clear();
    for (const [r, c] of stats.cells) {
      liveCellsSet.add(`${r},${c}`);
    }

    statGen.textContent = stats.generation.toLocaleString();
    statPop.textContent = stats.liveCount.toLocaleString();
    statCycles.textContent = stats.totalCycles.toLocaleString();
    statLatency.textContent = `${stats.stepTimeMs} ms`;
    statWmes.textContent = stats.wmeCount.toLocaleString();
  }

  function startSimulation() {
    if (isRunning) return;
    isRunning = true;
    btnPlay.classList.add('active');
    iconPlay.classList.add('hidden');
    iconPause.classList.remove('hidden');
    playText.textContent = 'Pause';
    lastFrameTime = performance.now();
    animLoop(lastFrameTime);
  }

  function stopSimulation() {
    if (!isRunning) return;
    isRunning = false;
    btnPlay.classList.remove('active');
    iconPlay.classList.remove('hidden');
    iconPause.classList.add('hidden');
    playText.textContent = 'Start';
    if (animFrameId) {
      cancelAnimationFrame(animFrameId);
      animFrameId = null;
    }
  }

  function animLoop(timestamp) {
    if (!isRunning) return;

    const interval = 1000 / targetFps;
    const elapsed = timestamp - lastFrameTime;

    if (elapsed >= interval) {
      stepSimulation();
      lastFrameTime = timestamp - (elapsed % interval);
    }

    animFrameId = requestAnimationFrame(animLoop);
  }

  function loadPresetPattern(name) {
    stopSimulation();
    const startR = Math.max(0, Math.floor(gridHeight / 2) - 4);
    const startC = Math.max(0, Math.floor(gridWidth / 2) - 5);
    const stats = window.lifeLoadPattern(name, startR, startC);
    if (stats.success) {
      updateStatsFromWasm(stats);
      drawGrid();
    }
  }

  // ----------------------------------------------------------------------------
  // Mouse & Touch Interaction
  // ----------------------------------------------------------------------------
  function getCellFromMouseEvent(e) {
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    const c = Math.floor(x / cellSize);
    const r = Math.floor(y / cellSize);
    if (r >= 0 && r < gridHeight && c >= 0 && c < gridWidth) {
      return { r, c };
    }
    return null;
  }

  function handlePointerDown(e) {
    const cell = getCellFromMouseEvent(e);
    if (!cell) return;

    isMouseDown = true;
    const key = `${cell.r},${cell.c}`;
    lastPaintedKey = key;

    // Toggle on click, or set paint mode
    const isCurrentlyAlive = liveCellsSet.has(key);
    paintMode = !isCurrentlyAlive;

    window.lifeSetCell(cell.r, cell.c, paintMode);
    if (paintMode) {
      liveCellsSet.add(key);
    } else {
      liveCellsSet.delete(key);
    }
    updateStatsFromWasm(window.lifeGetState());
    drawGrid();
  }

  function handlePointerMove(e) {
    const cell = getCellFromMouseEvent(e);
    if (cell) {
      hoverCoord.textContent = `Row: ${cell.r}, Col: ${cell.c}`;
    } else {
      hoverCoord.textContent = 'Row: -, Col: -';
    }

    if (!isMouseDown || !cell) return;

    const key = `${cell.r},${cell.c}`;
    if (key === lastPaintedKey) return;
    lastPaintedKey = key;

    window.lifeSetCell(cell.r, cell.c, paintMode);
    if (paintMode) {
      liveCellsSet.add(key);
    } else {
      liveCellsSet.delete(key);
    }
    updateStatsFromWasm(window.lifeGetState());
    drawGrid();
  }

  function handlePointerUp() {
    isMouseDown = false;
    lastPaintedKey = null;
  }

  // ----------------------------------------------------------------------------
  // UI Event Handlers
  // ----------------------------------------------------------------------------
  function setupEventListeners() {
    btnPlay.addEventListener('click', () => {
      if (isRunning) stopSimulation();
      else startSimulation();
    });

    btnStep.addEventListener('click', () => {
      stopSimulation();
      stepSimulation();
    });

    btnClear.addEventListener('click', () => {
      stopSimulation();
      window.lifeClear();
      updateStatsFromWasm(window.lifeGetState());
      drawGrid();
    });

    patternSelect.addEventListener('change', (e) => {
      loadPresetPattern(e.target.value);
    });

    gridSizeSelect.addEventListener('change', (e) => {
      stopSimulation();
      const size = parseInt(e.target.value, 10);
      initGrid(size, size);
      if (patternSelect.value) {
        loadPresetPattern(patternSelect.value);
      }
    });

    speedSlider.addEventListener('input', (e) => {
      targetFps = parseInt(e.target.value, 10);
      speedLabel.textContent = `${targetFps} FPS`;
    });

    // Canvas pointer events
    canvas.addEventListener('mousedown', handlePointerDown);
    window.addEventListener('mousemove', handlePointerMove);
    window.addEventListener('mouseup', handlePointerUp);

    canvas.addEventListener('touchstart', (e) => {
      e.preventDefault();
      const touch = e.touches[0];
      handlePointerDown(touch);
    }, { passive: false });

    canvas.addEventListener('touchmove', (e) => {
      e.preventDefault();
      const touch = e.touches[0];
      handlePointerMove(touch);
    }, { passive: false });

    window.addEventListener('touchend', handlePointerUp);

    // Keyboard shortcuts
    window.addEventListener('keydown', (e) => {
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
      if (e.code === 'Space') {
        e.preventDefault();
        btnPlay.click();
      } else if (e.code === 'KeyS') {
        e.preventDefault();
        btnStep.click();
      } else if (e.code === 'KeyC') {
        e.preventDefault();
        btnClear.click();
      }
    });

    // Rule inspector toggle & hot-reload
    btnToggleInspector.addEventListener('click', () => {
      const isHidden = inspectorDrawer.classList.contains('hidden');
      if (isHidden) {
        if (!ruleSourceEditor.value && typeof window.lifeGetRuleSource === 'function') {
          ruleSourceEditor.value = window.lifeGetRuleSource();
        }
        inspectorDrawer.classList.remove('hidden');
      } else {
        inspectorDrawer.classList.add('hidden');
      }
    });

    btnApplyRules.addEventListener('click', () => {
      stopSimulation();
      const customRules = ruleSourceEditor.value;
      const res = window.lifeInit(gridWidth, gridHeight, customRules);
      if (res.success) {
        rulesStatusMsg.textContent = '✓ Rules recompiled into Rete successfully!';
        rulesStatusMsg.className = 'rules-status success';
        if (patternSelect.value) {
          loadPresetPattern(patternSelect.value);
        } else {
          drawGrid();
        }
      } else {
        rulesStatusMsg.textContent = `Error: ${res.error}`;
        rulesStatusMsg.className = 'rules-status error';
      }
      setTimeout(() => {
        rulesStatusMsg.textContent = '';
      }, 4000);
    });

    btnResetRules.addEventListener('click', () => {
      stopSimulation();
      if (typeof window.lifeGetRuleSource === 'function') {
        const defaultRules = window.lifeGetRuleSource();
        ruleSourceEditor.value = defaultRules;
        window.lifeInit(gridWidth, gridHeight, defaultRules);
        rulesStatusMsg.textContent = '✓ Reset to default Conway rules.';
        rulesStatusMsg.className = 'rules-status success';
        if (patternSelect.value) {
          loadPresetPattern(patternSelect.value);
        }
        setTimeout(() => {
          rulesStatusMsg.textContent = '';
        }, 4000);
      }
    });

    btnCloseInspector.addEventListener('click', () => {
      inspectorDrawer.classList.add('hidden');
    });

    window.addEventListener('resize', () => {
      resizeCanvas();
    });
  }

  // Bootstrap
  initWasm();
})();
