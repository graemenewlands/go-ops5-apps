// State Management
const state = {
  wasmReady: false,
  cluster: null,
  clientDC: 'dc1',
  coordinatorID: 'dc1-n1',
  queryType: 'read',
  consistencyLevel: 'LOCAL_QUORUM',
  lastResult: null,
  isExecuting: false,
  liveCoordinatorID: null, // Active coordinator for live background traffic
  wanConnected: true,
};

// DOM References
const elements = {
  loadingOverlay: document.getElementById('loading-overlay'),
  selectCoordinator: document.getElementById('select-coordinator'),
  dcToggleGroup: document.getElementById('dc-toggle-group'),
  queryTypeGroup: document.getElementById('query-type-group'),
  clGroup: document.getElementById('cl-group'),
  btnExecuteQuery: document.getElementById('btn-execute-query'),
  btnResetCluster: document.getElementById('btn-reset-cluster'),
  btnToggleRules: document.getElementById('btn-toggle-rules'),
  ruleDrawer: document.getElementById('rule-drawer'),
  drawerBackdrop: document.getElementById('drawer-backdrop'),
  btnCloseDrawer: document.getElementById('btn-close-drawer'),
  ruleCodeBlock: document.getElementById('rule-code-block'),
  ringsSvg: document.getElementById('rings-svg'),
  clientLayer: document.getElementById('client-layer'),
  nodesLayer: document.getElementById('nodes-layer'),
  packetsLayer: document.getElementById('packets-layer'),
  wanLinkLayer: document.getElementById('wan-link-layer'),
  wanTitleText: document.getElementById('wan-title-text'),
  wanSubText: document.getElementById('wan-sub-text'),
  legendWanBadge: document.getElementById('legend-wan-badge'),
  terminalBody: document.getElementById('terminal-body'),
  btnClearTerminal: document.getElementById('btn-clear-terminal'),
  resultSourceBadge: document.getElementById('result-source-badge'),
  resultStatusBadge: document.getElementById('result-status-badge'),
  statCL: document.getElementById('stat-cl'),
  statLocalAcks: document.getElementById('stat-local-acks'),
  statTotalAcks: document.getElementById('stat-total-acks'),
  statLocalAlive: document.getElementById('stat-local-alive'),
  statTotalAlive: document.getElementById('stat-total-alive'),
  statCycles: document.getElementById('stat-cycles'),
  quorumProgressBar: document.getElementById('quorum-progress-bar'),
  quorumPercentText: document.getElementById('quorum-percent-text'),
  protocolEventsBox: document.getElementById('protocol-events-box'),
  nodeModal: document.getElementById('node-modal'),
  modalNodeTitle: document.getElementById('modal-node-title'),
  modalNodeBody: document.getElementById('modal-node-body'),
  btnCloseModal: document.getElementById('btn-close-modal'),
  toast: document.getElementById('toast'),
  btnToggleTraffic: document.getElementById('btn-toggle-traffic'),
  trafficBeacon: document.getElementById('traffic-beacon'),
  trafficBtnText: document.getElementById('traffic-btn-text'),
  trafficStatusBadge: document.getElementById('traffic-status-badge'),
  trafficStatusText: document.getElementById('traffic-status-text'),
};

// Ring Topology Geometry
const RING_GEOMETRY = {
  dc1: { cx: 310, cy: 220, r: 150 },
  dc2: { cx: 790, cy: 220, r: 150 },
  nodeRadius: 28,
};

// WebAssembly Initialization Callback
window.onOps5CassandraReady = () => {
  console.log("==> OPS5 Cassandra Protocol Engine WebAssembly Ready");
  state.wasmReady = true;
  elements.loadingOverlay.classList.add('hidden');
  initCluster();

  // Automatically start live background cluster traffic across DC1 and DC2
  setTimeout(() => {
    startAutoTraffic();
  }, 1000);
};

async function initWasm() {
  const go = new Go();
  try {
    const result = await WebAssembly.instantiateStreaming(
      fetch('main.wasm'),
      go.importObject
    );
    go.run(result.instance);
  } catch (err) {
    console.error("Failed to load Wasm binary:", err);
    elements.loadingOverlay.querySelector('.loading-text').innerText = "Failed to load WebAssembly";
    elements.loadingOverlay.querySelector('.loading-subtext').innerText = err.message;
  }
}

// Cluster State Management
function initCluster() {
  if (!state.wasmReady) return;
  const res = window.cassandraInit();
  if (res && res.success) {
    applyClusterState(res);
    logToTerminal("// Cluster initialized successfully with 2 DCs and 12 nodes.", "text-info");
  }
}

function applyClusterState(res) {
  if (res.cluster) state.cluster = res.cluster;
  if (res.clientDc) state.clientDC = res.clientDc;
  if (res.coordinatorId) state.coordinatorID = res.coordinatorId;
  if (res.wanConnected !== undefined) {
    state.wanConnected = res.wanConnected;
  }

  updateControlsUI();
  renderRings();
  renderWANLinkUI();
}

function renderWANLinkUI() {
  const isConnected = state.wanConnected;
  if (elements.wanLinkLayer) {
    elements.wanLinkLayer.classList.toggle('severed', !isConnected);
  }
  if (elements.wanTitleText) {
    elements.wanTitleText.textContent = isConnected ? '⚡ WAN Link' : '✂️ WAN SEVERED';
  }
  if (elements.wanSubText) {
    elements.wanSubText.textContent = isConnected
      ? 'CONNECTED (Click to Sever)'
      : 'PARTITION ACTIVE (Click to Reconnect)';
  }
  if (elements.legendWanBadge) {
    elements.legendWanBadge.classList.toggle('severed', !isConnected);
    elements.legendWanBadge.textContent = isConnected
      ? '⚡ WAN: CONNECTED'
      : '✂️ WAN: SEVERED (PARTITION)';
  }
}

function toggleWANConnection() {
  if (!state.wasmReady) return;
  const res = window.cassandraToggleWAN();
  applyClusterState(res);
  const isConnected = state.wanConnected;
  if (isConnected) {
    logToTerminal("[WAN LINK] Inter-datacenter WAN link RESTORED. Cross-DC replication re-enabled.", "text-success");
    showToast("WAN Link Restored (Connected)");
  } else {
    logToTerminal("[WAN LINK] Inter-datacenter WAN link SEVERED! Network partition active between DC1 and DC2.", "text-danger");
    showToast("WAN Link Severed (Partition Active)");
  }
}

function updateControlsUI() {
  // Update DC segmented toggle
  elements.dcToggleGroup.querySelectorAll('.btn-segmented').forEach(b => {
    b.classList.toggle('active', b.dataset.dc === state.clientDC);
  });

  // Populate coordinator dropdown
  elements.selectCoordinator.innerHTML = '';
  const currentDC = state.cluster.dcs.find(dc => dc.id === state.clientDC);
  if (currentDC) {
    currentDC.nodes.forEach(n => {
      const opt = document.createElement('option');
      opt.value = n.id;
      const stateCode = `${n.health}${n.membership}`;
      const replicaBadge = n.isReplica ? ' [Replica]' : '';
      opt.innerText = `${n.id} (${stateCode})${replicaBadge}`;
      if (n.id === state.coordinatorID) {
        opt.selected = true;
      }
      elements.selectCoordinator.appendChild(opt);
    });
  }
}

// Render SVG Ring Topology
function renderRings() {
  elements.nodesLayer.innerHTML = '';
  if (!state.cluster) return;

  state.cluster.dcs.forEach(dc => {
    const geom = RING_GEOMETRY[dc.id];
    const nodeCount = dc.nodes.length; // 6 nodes

    dc.nodes.forEach((n, idx) => {
      // 60-degree increments starting from top (-90 degrees)
      const angle = ((idx * 60) - 90) * (Math.PI / 180);
      const nx = geom.cx + geom.r * Math.cos(angle);
      const ny = geom.cy + geom.r * Math.sin(angle);

      renderNodeSVG(n, nx, ny, dc.id);
    });
  });

  renderClientMarker();
}

function renderClientMarker() {
  const clientLayer = elements.clientLayer || document.getElementById('client-layer');
  if (!clientLayer) return;
  clientLayer.innerHTML = '';

  const dcId = state.clientDC;
  const geom = RING_GEOMETRY[dcId] || RING_GEOMETRY.dc1;
  const clientX = geom.cx;
  const clientY = 460;

  // Find coordinator coords
  const coordPos = getNodeCoords(state.coordinatorID);
  if (coordPos) {
    const link = document.createElementNS('http://www.w3.org/2000/svg', 'line');
    link.setAttribute('x1', clientX);
    link.setAttribute('y1', clientY - 18);
    link.setAttribute('x2', coordPos.x);
    link.setAttribute('y2', coordPos.y + 28);
    link.setAttribute('class', 'client-link');
    clientLayer.appendChild(link);
  }

  // Client Box Group
  const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  g.setAttribute('class', 'client-box');

  const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
  rect.setAttribute('x', clientX - 85);
  rect.setAttribute('y', clientY - 16);
  rect.setAttribute('width', 170);
  rect.setAttribute('height', 32);
  rect.setAttribute('rx', 8);
  rect.setAttribute('class', 'client-rect');
  g.appendChild(rect);

  const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  text.setAttribute('x', clientX);
  text.setAttribute('y', clientY + 4);
  text.setAttribute('text-anchor', 'middle');
  text.setAttribute('class', 'client-text');
  text.textContent = `💻 Client → ${state.clientDC === 'dc1' ? 'DC1 (East)' : 'DC2 (West)'}`;
  g.appendChild(text);

  clientLayer.appendChild(g);
}

function getClientCoords() {
  const geom = RING_GEOMETRY[state.clientDC] || RING_GEOMETRY.dc1;
  return { x: geom.cx, y: 460 - 18 };
}

function renderNodeSVG(node, x, y, dcId) {
  const stateCode = `${node.health}${node.membership}`;
  const isClientCoord = (node.id === state.coordinatorID);
  const isLiveCoord = (state.liveCoordinatorID === node.id);

  let extraClasses = '';
  if (isClientCoord) extraClasses += ' node-coord';
  if (isLiveCoord) extraClasses += ' node-live-coord';

  const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  g.setAttribute('class', `node-g node-${stateCode.toLowerCase()}${extraClasses}`);
  g.setAttribute('id', `node-el-${node.id}`);
  g.dataset.nodeId = node.id;

  // Node Circle
  const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  circle.setAttribute('cx', x);
  circle.setAttribute('cy', y);
  circle.setAttribute('r', RING_GEOMETRY.nodeRadius);
  circle.setAttribute('class', 'node-circle');
  if (node.isReplica) {
    circle.setAttribute('filter', 'url(#glow-replica)');
  }
  g.appendChild(circle);

  // Node ID label (e.g. dc1-n1)
  const textId = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  textId.setAttribute('x', x);
  textId.setAttribute('y', isClientCoord ? y - 4 : y - 6);
  textId.setAttribute('text-anchor', 'middle');
  textId.setAttribute('class', 'node-label');
  textId.textContent = node.id.replace('dc1-', 'N').replace('dc2-', 'N');
  g.appendChild(textId);

  // State Code Pill (UN, UJ, DS, DN)
  const textState = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  textState.setAttribute('x', x);
  textState.setAttribute('y', isClientCoord ? y + 10 : y + 12);
  textState.setAttribute('text-anchor', 'middle');
  textState.setAttribute('class', 'node-state-pill');
  textState.setAttribute('fill', getStateColor(stateCode));
  textState.textContent = stateCode;
  g.appendChild(textState);

  // Coordinator Crown / Badge
  if (isClientCoord) {
    const coordPill = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    coordPill.setAttribute('x', x);
    coordPill.setAttribute('y', y + 23);
    coordPill.setAttribute('text-anchor', 'middle');
    coordPill.setAttribute('fill', '#fbbf24');
    coordPill.setAttribute('font-size', '8px');
    coordPill.setAttribute('font-weight', '700');
    coordPill.textContent = '★ CLIENT COORD';
    g.appendChild(coordPill);
  } else if (isLiveCoord) {
    const liveCoordPill = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    liveCoordPill.setAttribute('x', x);
    liveCoordPill.setAttribute('y', y + 23);
    liveCoordPill.setAttribute('text-anchor', 'middle');
    liveCoordPill.setAttribute('fill', '#34d399');
    liveCoordPill.setAttribute('font-size', '8px');
    liveCoordPill.setAttribute('font-weight', '700');
    liveCoordPill.textContent = '⚡ LIVE COORD';
    g.appendChild(liveCoordPill);
  }

  // Replica Indicator Badge (cylinder/pill above node)
  if (node.isReplica) {
    const repPillBg = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
    repPillBg.setAttribute('x', x - 28);
    repPillBg.setAttribute('y', y - 44);
    repPillBg.setAttribute('width', 56);
    repPillBg.setAttribute('height', 14);
    repPillBg.setAttribute('rx', 4);
    repPillBg.setAttribute('fill', '#0284c7');
    repPillBg.setAttribute('opacity', '0.9');
    g.appendChild(repPillBg);

    const repText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    repText.setAttribute('x', x);
    repText.setAttribute('y', y - 34);
    repText.setAttribute('text-anchor', 'middle');
    repText.setAttribute('class', 'node-replica-indicator');
    repText.setAttribute('fill', '#ffffff');
    repText.textContent = 'REPLICA';
    g.appendChild(repText);
  }

  // Click handler to open node inspection modal
  g.addEventListener('click', (e) => {
    e.stopPropagation();
    openNodeModal(node);
  });

  elements.nodesLayer.appendChild(g);
}

function getStateColor(stateCode) {
  switch (stateCode) {
    case 'UN': return '#34d399';
    case 'UJ': return '#fbbf24';
    case 'DS': return '#f87171';
    case 'DN': return '#ef4444';
    default: return '#94a3b8';
  }
}

// Node Modal Inspection & Actions
function openNodeModal(node) {
  const stateCode = `${node.health}${node.membership}`;
  const isCoord = (node.id === state.coordinatorID);

  elements.modalNodeTitle.innerHTML = `
    <span>${node.id}</span>
    <span class="badge" style="background:${getStateColor(stateCode)}22;color:${getStateColor(stateCode)};">${stateCode}</span>
    ${node.isReplica ? '<span class="badge badge-wasm">Target Replica</span>' : ''}
    ${isCoord ? '<span class="badge" style="background:#fbbf2422;color:#fbbf24;">Coordinator</span>' : ''}
  `;

  elements.modalNodeBody.innerHTML = `
    <div style="font-size:0.875rem;line-height:1.7;">
      <div><strong>Datacenter:</strong> <code>${node.dc === 'dc1' ? 'DC1 (East)' : 'DC2 (West)'}</code></div>
      <div><strong>Ring Position:</strong> Node ${node.ringPos} of 6</div>
      <div><strong>Lifecycle State:</strong> <code>${describeState(stateCode)}</code></div>
      <div><strong>Target Replica:</strong> ${node.isReplica ? 'Yes (Holds 50% dataset partition)' : 'No'}</div>
      <div><strong>Stored Value:</strong> <code>${node.value || 'none'}</code></div>
      <div><strong>Timestamp:</strong> <code>${node.timestamp || 0}</code></div>
    </div>

    <div style="display:flex;gap:10px;margin-top:14px;flex-wrap:wrap;">
      <button id="modal-btn-pull-plug" class="btn btn-secondary btn-sm" style="border-color:#ef4444;color:#f87171;">
        🔌 Pull Plug (Set DS)
      </button>
      <button id="modal-btn-cycle-state" class="btn btn-secondary btn-sm" style="border-color:#fbbf24;color:#fbbf24;">
        ⚡ Cycle State (DS &rarr; UJ &rarr; UN)
      </button>
      ${!isCoord ? '<button id="modal-btn-set-coord" class="btn btn-primary btn-sm">⭐ Set Coordinator</button>' : ''}
    </div>
  `;

  // Attach modal action button handlers
  const btnPull = document.getElementById('modal-btn-pull-plug');
  if (btnPull) {
    btnPull.onclick = () => {
      const res = window.cassandraPullPlug(node.id);
      applyClusterState(res);
      closeNodeModal();
      logToTerminal(`[FAULT] Pulled plug on node ${node.id} &rarr; State is now DS (Down/Stopped)`, "text-danger");
      showToast(`Pulled plug on ${node.id} (State: DS)`);
    };
  }

  const btnCycle = document.getElementById('modal-btn-cycle-state');
  if (btnCycle) {
    btnCycle.onclick = () => {
      const res = window.cassandraCycleNodeState(node.id);
      applyClusterState(res);
      closeNodeModal();
      logToTerminal(`[LIFECYCLE] Cycled state of node ${node.id} &rarr; State is now ${res.cycledState}`, "text-warn");
      showToast(`Cycled ${node.id} to ${res.cycledState}`);
    };
  }

  const btnCoord = document.getElementById('modal-btn-set-coord');
  if (btnCoord) {
    btnCoord.onclick = () => {
      const res = window.cassandraSetCoordinator(node.id);
      applyClusterState(res);
      closeNodeModal();
      logToTerminal(`[CLIENT] Client coordinator set to ${node.id} in ${node.dc}`, "text-info");
      showToast(`Set coordinator to ${node.id}`);
    };
  }

  elements.nodeModal.classList.add('open');
}

function describeState(code) {
  switch (code) {
    case 'UN': return 'Up & Normal (Operational replica serving traffic)';
    case 'UJ': return 'Up & Joining (Bootstrap / token streaming active)';
    case 'DS': return 'Down & Stopped (Process stopped / plug pulled)';
    case 'DN': return 'Down & Normal (Process crashed / unreachable)';
    default: return code;
  }
}

function closeNodeModal() {
  elements.nodeModal.classList.remove('open');
}

elements.btnCloseModal.onclick = closeNodeModal;
elements.nodeModal.onclick = (e) => {
  if (e.target === elements.nodeModal) closeNodeModal();
};

// Coordinator Selection Handler
elements.selectCoordinator.addEventListener('change', (e) => {
  const nodeID = e.target.value;
  const res = window.cassandraSetCoordinator(nodeID);
  applyClusterState(res);
  logToTerminal(`[CLIENT] Selected coordinator ${nodeID}`, "text-info");
});

// DC Toggle Handler
elements.dcToggleGroup.querySelectorAll('.btn-segmented').forEach(b => {
  b.addEventListener('click', () => {
    const dcID = b.dataset.dc;
    const res = window.cassandraSetClientDC(dcID);
    applyClusterState(res);
    logToTerminal(`[CLIENT] Connected client to Datacenter ${dcID === 'dc1' ? '1 (East)' : '2 (West)'}`, "text-info");
  });
});

// Query Type Toggle Handler
elements.queryTypeGroup.querySelectorAll('.btn-segmented').forEach(b => {
  b.addEventListener('click', () => {
    elements.queryTypeGroup.querySelectorAll('.btn-segmented').forEach(x => x.classList.remove('active'));
    b.classList.add('active');
    state.queryType = b.dataset.type;
  });
});

// Consistency Level Toggle Handler
elements.clGroup.querySelectorAll('.cl-btn').forEach(b => {
  b.addEventListener('click', () => {
    elements.clGroup.querySelectorAll('.cl-btn').forEach(x => x.classList.remove('active'));
    b.classList.add('active');
    state.consistencyLevel = b.dataset.cl;
    elements.statCL.innerText = state.consistencyLevel;
  });
});

// Reset Cluster Handler
elements.btnResetCluster.onclick = () => {
  const res = window.cassandraReset();
  applyClusterState(res);
  resetMetrics();
  logToTerminal("[CLUSTER] Cluster reset to default pristine state (all 12 nodes UN).", "text-info");
  showToast("Cluster reset to default state");
};

// WAN Link Toggle Handlers (Center SVG button & Legend badge)
if (elements.wanLinkLayer) {
  elements.wanLinkLayer.onclick = toggleWANConnection;
  elements.wanLinkLayer.onkeydown = (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      toggleWANConnection();
    }
  };
}
if (elements.legendWanBadge) {
  elements.legendWanBadge.onclick = toggleWANConnection;
}

function resetMetrics() {
  elements.resultStatusBadge.className = 'badge badge-success';
  elements.resultStatusBadge.innerText = 'Ready';
  elements.statLocalAcks.innerText = '0 / 2';
  elements.statTotalAcks.innerText = '0 / 2';
  elements.statLocalAlive.innerText = '3 / 3';
  elements.statTotalAlive.innerText = '6 / 6';
  elements.statCycles.innerText = '0';
  elements.quorumProgressBar.style.width = '0%';
  elements.quorumPercentText.innerText = '0%';
  elements.protocolEventsBox.innerHTML = '';
}

// Auto-Traffic Generator State
const trafficState = {
  isRunning: false,
  timerId: null,
  countdownId: null,
  nextTickTime: 0,
  nextDC: 'dc1', // Alternates between dc1 and dc2
};

const CONSISTENCY_LEVELS = ['ONE', 'TWO', 'THREE', 'LOCAL_QUORUM', 'QUORUM'];
const QUERY_TYPES = ['read', 'read', 'write']; // 67% reads, 33% writes typical Cassandra workload

// Box-Muller Gaussian / Normal Distribution Sampling (mean ~5000ms, stdDev ~1200ms)
function sampleNormalInterval(mean = 5000, stdDev = 1200) {
  let u = 0, v = 0;
  while (u === 0) u = Math.random();
  while (v === 0) v = Math.random();
  const z = Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v);
  // Clamped between 2000ms and 8500ms
  return Math.max(2000, Math.min(8500, Math.round(mean + z * stdDev)));
}

function toggleAutoTraffic() {
  if (trafficState.isRunning) {
    stopAutoTraffic();
  } else {
    startAutoTraffic();
  }
}

function startAutoTraffic() {
  if (trafficState.isRunning) return;
  if (!state.wasmReady || !state.cluster) {
    return;
  }
  trafficState.isRunning = true;

  elements.btnToggleTraffic.classList.add('active');
  elements.trafficBeacon.classList.add('active');
  elements.trafficBtnText.innerText = "Pause Live Traffic";
  elements.trafficStatusBadge.classList.add('active');

  logToTerminal("[TRAFFIC GENERATOR] Live background cluster traffic running across DC1 and DC2 (~5.0s normal distribution).", "text-warn");

  // Schedule first tick after 1.2s lead-in
  scheduleNextTraffic(1200);
  startCountdownTimer();
}

function stopAutoTraffic() {
  if (!trafficState.isRunning) return;
  trafficState.isRunning = false;

  if (trafficState.timerId) {
    clearTimeout(trafficState.timerId);
    trafficState.timerId = null;
  }
  if (trafficState.countdownId) {
    clearInterval(trafficState.countdownId);
    trafficState.countdownId = null;
  }

  elements.btnToggleTraffic.classList.remove('active');
  elements.trafficBeacon.classList.remove('active');
  elements.trafficBtnText.innerText = "Resume Live Traffic";
  elements.trafficStatusBadge.classList.remove('active');
  elements.trafficStatusText.innerText = "Live Traffic: Paused";

  logToTerminal("[TRAFFIC GENERATOR] Live background traffic paused.", "text-muted");
  showToast("Live traffic paused");
}

function scheduleNextTraffic(delayMs) {
  if (!trafficState.isRunning) return;
  trafficState.nextTickTime = Date.now() + delayMs;
  trafficState.timerId = setTimeout(async () => {
    await executeAutoTrafficTick();
  }, delayMs);
}

function startCountdownTimer() {
  if (trafficState.countdownId) clearInterval(trafficState.countdownId);
  trafficState.countdownId = setInterval(() => {
    if (!trafficState.isRunning) return;
    const remaining = Math.max(0, (trafficState.nextTickTime - Date.now()) / 1000);
    const dcLabel = (trafficState.nextDC === 'dc1') ? 'DC1' : 'DC2';
    elements.trafficStatusText.innerHTML = `Live: <strong>Active (~5s)</strong> &bull; Next: <strong>${dcLabel}</strong> in ${remaining.toFixed(1)}s`;
  }, 200);
}

async function executeAutoTrafficTick() {
  if (!trafficState.isRunning) return;

  if (state.isExecuting) {
    // If a manual client query is currently running, retry shortly
    scheduleNextTraffic(800);
    return;
  }

  // 1. Alternate between DC1 and DC2
  const targetDC = trafficState.nextDC;
  trafficState.nextDC = (targetDC === 'dc1') ? 'dc2' : 'dc1';

  // 2. Select coordinator node in chosen DC (prefer live nodes)
  const dcObj = state.cluster && state.cluster.dcs.find(d => d.id === targetDC);
  let coordID = `${targetDC}-n1`;
  if (dcObj && dcObj.nodes.length > 0) {
    const liveNodes = dcObj.nodes.filter(n => n.health === 'U' && n.membership !== 'S');
    const candidates = liveNodes.length > 0 ? liveNodes : dcObj.nodes;
    const picked = candidates[Math.floor(Math.random() * candidates.length)];
    coordID = picked.id;
  }

  // 3. Random Consistency Level and Query Type
  const chosenCL = CONSISTENCY_LEVELS[Math.floor(Math.random() * CONSISTENCY_LEVELS.length)];
  const chosenType = QUERY_TYPES[Math.floor(Math.random() * QUERY_TYPES.length)];

  // Execute live background query without modifying user's client selection
  await executeQuery({
    queryType: chosenType,
    consistencyLevel: chosenCL,
    dc: targetDC,
    coordinatorID: coordID,
    isAuto: true,
  });

  // Schedule next execution using Gaussian / Normal distribution
  if (trafficState.isRunning) {
    const nextDelay = sampleNormalInterval(5000, 1200);
    scheduleNextTraffic(nextDelay);
  }
}

elements.btnToggleTraffic.onclick = toggleAutoTraffic;

// Unified Execute Query function (Client queries vs Live background traffic)
async function executeQuery(options = {}) {
  if (state.isExecuting) return null;
  state.isExecuting = true;
  elements.btnExecuteQuery.disabled = true;

  const isAuto = options.isAuto || false;

  try {
    let qType, cl, targetDC, coordID;

    if (isAuto) {
      // Live traffic uses its own assigned parameters without disturbing client form controls
      qType = options.queryType || 'read';
      cl = options.consistencyLevel || 'LOCAL_QUORUM';
      targetDC = options.dc || 'dc1';
      coordID = options.coordinatorID || `${targetDC}-n1`;

      state.liveCoordinatorID = coordID;
      renderRings(); // Highlight live coordinator in SVG
    } else {
      // User Client query uses the client connection state
      qType = state.queryType;
      cl = state.consistencyLevel;
      targetDC = state.clientDC;
      coordID = state.coordinatorID;
    }

    const key = options.key || "users_dataset";
    const writeVal = (qType === 'write')
      ? (options.writeVal || `user_${Math.floor(1000 + Math.random() * 9000)}`)
      : '';

    const dcLabel = (targetDC === 'dc1') ? 'DC1 (East)' : 'DC2 (West)';
    logToTerminal(`\n------------------------------------------------------------`, "text-muted");
    if (isAuto) {
      logToTerminal(`[LIVE TRAFFIC - ${dcLabel}] Background query executing: Type=${qType.toUpperCase()} CL=${cl} Coordinator=${coordID}`, "text-warn");
    } else {
      logToTerminal(`[CLIENT REQUEST] User Client (${dcLabel}) sending query: Type=${qType.toUpperCase()} Key=${key} CL=${cl} Coordinator=${coordID}`, "text-info");
    }

    // Call OPS5 WebAssembly Rule Engine (passing coordID override)
    const res = window.cassandraExecuteQuery(qType, cl, key, writeVal, coordID);
    state.lastResult = res;

    if (res.cluster) {
      state.cluster = res.cluster;
      renderRings();
    }

    // Animate packet flights across SVG (includes Client <-> Coord flight if client query)
    await animateQueryPackets(res, !isAuto);

    // Render protocol results
    renderQueryResult(res, isAuto ? 'live' : 'client');

    return res;
  } catch (err) {
    console.error("Query execution error:", err);
    logToTerminal(`[ERROR] Query execution failed: ${err.message}`, "text-danger");
    return null;
  } finally {
    state.liveCoordinatorID = null;
    renderRings();
    state.isExecuting = false;
    elements.btnExecuteQuery.disabled = false;
  }
}

// Manual Execute Query Button Handler (User Client Request)
elements.btnExecuteQuery.onclick = async () => {
  await executeQuery({
    queryType: state.queryType,
    consistencyLevel: state.consistencyLevel,
    dc: state.clientDC,
    coordinatorID: state.coordinatorID,
    isAuto: false,
  });
};

// Animated Packet Flights in SVG
async function animateQueryPackets(res, isClientQuery = false) {
  elements.packetsLayer.innerHTML = '';
  const coordNode = getNodeCoords(res.coordinator);
  if (!coordNode || !res.messages || res.messages.length === 0) return;

  // 1. If User Client query, animate request packet from Client marker to Coordinator!
  if (isClientQuery) {
    const clientPos = getClientCoords();
    createPacketAnimation(clientPos, coordNode, '#fbbf24', 300);
    await sleep(320);
  }

  // 2. Dispatch flights from coordinator to replica nodes
  const dispatchPackets = [];
  res.messages.forEach(m => {
    if (m.kind === 'request') {
      const destCoords = getNodeCoords(m.toNode);
      if (destCoords) {
        if (!state.wanConnected && m.isLocal === false) {
          // Cross-DC dispatch is severed at WAN link boundary (550, 220)
          const wanBoundaryCoords = { x: 550, y: 220 };
          dispatchPackets.push({ from: coordNode, to: wanBoundaryCoords, msg: m, severed: true });
        } else {
          dispatchPackets.push({ from: coordNode, to: destCoords, msg: m, severed: false });
        }
      }
    }
  });

  if (dispatchPackets.length > 0) {
    const dispatchColor = isClientQuery ? '#38bdf8' : '#818cf8';
    dispatchPackets.forEach(p => {
      const col = p.severed ? '#ef4444' : dispatchColor;
      createPacketAnimation(p.from, p.to, col, p.severed ? 280 : 400);
    });
    await sleep(420);
  }

  // 3. Response flights from responding nodes back to coordinator
  const responsePackets = [];
  res.messages.forEach(m => {
    if (m.kind === 'response' && m.status === 'delivered') {
      const sourceCoords = getNodeCoords(m.fromNode);
      if (sourceCoords) {
        responsePackets.push({ from: sourceCoords, to: coordNode, msg: m });
      }
    }
  });

  if (responsePackets.length > 0) {
    responsePackets.forEach(p => {
      createPacketAnimation(p.from, p.to, '#34d399', 400);
    });
    await sleep(420);
  }

  // 4. If User Client query, animate ack response packet from Coordinator back to Client!
  if (isClientQuery) {
    const clientPos = getClientCoords();
    createPacketAnimation(coordNode, clientPos, '#fbbf24', 300);
    await sleep(320);
  }

  elements.packetsLayer.innerHTML = '';
}

function getNodeCoords(nodeID) {
  if (!state.cluster) return null;
  for (const dc of state.cluster.dcs) {
    const geom = RING_GEOMETRY[dc.id];
    const idx = dc.nodes.findIndex(n => n.id === nodeID);
    if (idx !== -1) {
      const angle = ((idx * 60) - 90) * (Math.PI / 180);
      return {
        x: geom.cx + geom.r * Math.cos(angle),
        y: geom.cy + geom.r * Math.sin(angle),
      };
    }
  }
  return null;
}

function createPacketAnimation(p1, p2, color, durationMs) {
  const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  circle.setAttribute('r', '5');
  circle.setAttribute('fill', color);
  circle.setAttribute('filter', 'url(#glow-coord)');
  circle.setAttribute('cx', p1.x);
  circle.setAttribute('cy', p1.y);
  elements.packetsLayer.appendChild(circle);

  const trail = document.createElementNS('http://www.w3.org/2000/svg', 'line');
  trail.setAttribute('x1', p1.x);
  trail.setAttribute('y1', p1.y);
  trail.setAttribute('x2', p2.x);
  trail.setAttribute('y2', p2.y);
  trail.setAttribute('class', 'packet-trail');
  trail.setAttribute('stroke', color);
  elements.packetsLayer.appendChild(trail);

  // Simple keyframe interpolation
  const startTime = performance.now();
  function animate(now) {
    const elapsed = now - startTime;
    const progress = Math.min(elapsed / durationMs, 1);
    const cx = p1.x + (p2.x - p1.x) * progress;
    const cy = p1.y + (p2.y - p1.y) * progress;
    circle.setAttribute('cx', cx);
    circle.setAttribute('cy', cy);

    if (progress < 1) {
      requestAnimationFrame(animate);
    }
  }
  requestAnimationFrame(animate);
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// Render Query Result
function renderQueryResult(res, origin = 'client') {
  // Query Origin Source Badge
  if (elements.resultSourceBadge) {
    if (origin === 'live') {
      elements.resultSourceBadge.className = 'badge badge-live';
      elements.resultSourceBadge.innerText = `LIVE TRAFFIC [${res.localDc.toUpperCase()}]`;
    } else {
      elements.resultSourceBadge.className = 'badge badge-client';
      elements.resultSourceBadge.innerText = 'CLIENT REQUEST (YOU)';
    }
  }

  // Status Badge
  if (res.success) {
    elements.resultStatusBadge.className = 'badge badge-success';
    elements.resultStatusBadge.innerText = 'QUORUM SATISFIED';
  } else {
    elements.resultStatusBadge.className = 'badge badge-danger';
    elements.resultStatusBadge.innerText = 'CONSISTENCY FAILED';
  }

  // Quorum Metrics
  const reqTotal = getRequiredAcks(res.consistencyLevel, res.localDc);
  elements.statCL.innerText = res.consistencyLevel;
  elements.statLocalAcks.innerText = `${res.ackResult.localAcks} / 2`;
  elements.statTotalAcks.innerText = `${res.ackResult.totalAcks} / ${reqTotal}`;
  elements.statLocalAlive.innerText = `${res.liveTally.localAlive} / 3`;
  elements.statTotalAlive.innerText = `${res.liveTally.totalAlive} / 6`;
  elements.statCycles.innerText = res.stats.cycleCount;

  // Quorum Progress Bar
  const achieved = (res.consistencyLevel === 'LOCAL_QUORUM') ? res.ackResult.localAcks : res.ackResult.totalAcks;
  const pct = Math.min(Math.round((achieved / reqTotal) * 100), 100);
  elements.quorumProgressBar.style.width = `${pct}%`;
  elements.quorumPercentText.innerText = `${pct}% (${achieved}/${reqTotal})`;

  // Hinted Handoffs & Read Repairs
  elements.protocolEventsBox.innerHTML = '';
  if (res.hintedHandoffs && res.hintedHandoffs.length > 0) {
    res.hintedHandoffs.forEach(h => {
      const p = document.createElement('div');
      p.className = 'event-pill event-hint';
      p.innerHTML = `<strong>[HINTED HANDOFF]</strong> Coordinator saved hint for down replica <code>${h.targetNode}</code> (val="${h.value}")`;
      elements.protocolEventsBox.appendChild(p);
    });
  }

  if (res.readRepairs && res.readRepairs.length > 0) {
    res.readRepairs.forEach(r => {
      const p = document.createElement('div');
      p.className = 'event-pill event-repair';
      p.innerHTML = `<strong>[READ REPAIR]</strong> Outdated replica <code>${r.targetNode}</code> updated to ts=${r.repairedTs} (val="${r.repairedVal}")`;
      elements.protocolEventsBox.appendChild(p);
    });
  }

  // Detailed Terminal Logs
  logToTerminal(`[DISPATCH] Natural endpoints: 6 replicas (3 local in ${res.localDc}, 3 remote)`, "text-info");
  logToTerminal(`[PRE-FLIGHT] Live replicas tally: Local=${res.liveTally.localAlive}/3, Remote=${res.liveTally.remoteAlive}/3, Total=${res.liveTally.totalAlive}/6`, "text-info");

  if (!res.success) {
    logToTerminal(`[EXCEPTION] ${res.errorReason}`, "text-danger");
    logToTerminal(`[RESULT] Query FAILED in ${res.stats.elapsedMs.toFixed(2)}ms (${res.stats.cycleCount} OPS5 cycles)`, "text-danger");
    if (origin === 'client') showToast(`Query Failed: ${res.errorReason}`);
  } else {
    logToTerminal(`[ACKS] Coordinator received ${res.ackResult.totalAcks} total acks (${res.ackResult.localAcks} local in ${res.localDc})`, "text-success");
    if (res.queryType === 'read') {
      logToTerminal(`[VALUE RESOLUTION] Read resolved value="${res.resolvedValue}" (Timestamp=${res.highestTimestamp})`, "text-success");
    }
    logToTerminal(`[RESULT] Query SUCCESS at CL=${res.consistencyLevel} in ${res.stats.elapsedMs.toFixed(2)}ms (${res.stats.cycleCount} OPS5 cycles)`, "text-success");
    if (origin === 'client') showToast(`Query Success at ${res.consistencyLevel}!`);
  }
}

function getRequiredAcks(cl, localDc) {
  switch (cl) {
    case 'ONE': return 1;
    case 'TWO': return 2;
    case 'THREE': return 3;
    case 'LOCAL_QUORUM': return 2;
    case 'QUORUM': return 4;
    default: return 2;
  }
}

function logToTerminal(message, cssClass = '') {
  const line = document.createElement('div');
  line.className = `log-line ${cssClass}`;
  line.innerText = message;
  elements.terminalBody.appendChild(line);
  elements.terminalBody.scrollTop = elements.terminalBody.scrollHeight;
}

elements.btnClearTerminal.onclick = () => {
  elements.terminalBody.innerHTML = '';
  logToTerminal("// Trace cleared.", "text-muted");
};

// Slide-out Rule Drawer
elements.btnToggleRules.onclick = () => {
  const rules = window.cassandraGetRuleSource();
  elements.ruleCodeBlock.innerText = rules;
  elements.ruleDrawer.classList.add('open');
  elements.drawerBackdrop.classList.add('open');
};

elements.btnCloseDrawer.onclick = closeDrawer;
elements.drawerBackdrop.onclick = closeDrawer;

function closeDrawer() {
  elements.ruleDrawer.classList.remove('open');
  elements.drawerBackdrop.classList.remove('open');
}

// Toast notification helper
function showToast(message) {
  elements.toast.innerText = message;
  elements.toast.classList.add('show');
  setTimeout(() => {
    elements.toast.classList.remove('show');
  }, 2800);
}

// Start application
initWasm();
