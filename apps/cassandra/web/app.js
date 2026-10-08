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
  dcCount: 2,
  replicasPerDc: 3,
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
  btnAddDC: document.getElementById('btn-add-dc'),
  btnRemoveDC: document.getElementById('btn-remove-dc'),
  dcCountBadge: document.getElementById('dc-count-badge'),
  btnAddReplica: document.getElementById('btn-add-replica'),
  btnRemoveReplica: document.getElementById('btn-remove-replica'),
  replicaCountBadge: document.getElementById('replica-count-badge'),
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
  ringBaseDC1: document.getElementById('ring-base-dc1'),
  ringBaseDC2: document.getElementById('ring-base-dc2'),
  clusterEmptyMessage: document.getElementById('cluster-empty-message'),
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

// Datacenter Metadata
const DC_METADATA = {
  dc1: { name: 'Datacenter 1', region: 'US East' },
  dc2: { name: 'Datacenter 2', region: 'US West' },
  dc3: { name: 'Datacenter 3', region: 'EU Central' },
  dc4: { name: 'Datacenter 4', region: 'AP South' },
  dc5: { name: 'Datacenter 5', region: 'SA East' },
};

function getDCRegion(dcId) {
  return (DC_METADATA[dcId] && DC_METADATA[dcId].region) ? DC_METADATA[dcId].region : dcId.toUpperCase();
}

function getDCLabel(dcId) {
  if (!dcId) return 'No DC';
  const meta = DC_METADATA[dcId];
  return meta ? `${dcId.toUpperCase()} (${meta.region})` : dcId.toUpperCase();
}

const NODE_RADIUS = 28;
let currentLayout = null;

// Dynamic Ring Topology Geometry for 0 to 5 Datacenters
function computeLayout(dcs) {
  const dcCount = dcs ? dcs.length : 0;
  if (dcCount === 0) {
    return {
      svgWidth: 1100,
      geoms: {},
      wanLinks: [],
      wan: { x1: 0, x2: 0, midX: 550 },
    };
  }
  if (dcCount === 1) {
    const dcId = dcs[0].id;
    return {
      svgWidth: 800,
      geoms: {
        [dcId]: { cx: 400, cy: 220, r: 140 },
      },
      wanLinks: [],
      wan: { x1: 400, x2: 400, midX: 400 },
    };
  }
  if (dcCount === 2) {
    const geoms = {
      [dcs[0].id]: { cx: 310, cy: 220, r: 140 },
      [dcs[1].id]: { cx: 790, cy: 220, r: 140 },
    };
    const wanLinks = [
      {
        dc1: dcs[0].id,
        dc2: dcs[1].id,
        x1: 310 + 140,
        x2: 790 - 140,
        midX: 550,
        cy: 220,
      },
    ];
    return {
      svgWidth: 1100,
      geoms,
      wanLinks,
      wan: { x1: 310, x2: 790, midX: 550 },
    };
  }

  // 3, 4, or 5 Datacenters
  const spacing = 380;
  const margin = 210;
  const svgWidth = margin * 2 + (dcCount - 1) * spacing;
  const geoms = {};
  dcs.forEach((dc, i) => {
    geoms[dc.id] = {
      cx: margin + i * spacing,
      cy: 220,
      r: 130,
    };
  });
  const wanLinks = [];
  for (let i = 0; i < dcCount - 1; i++) {
    const d1 = dcs[i].id;
    const d2 = dcs[i + 1].id;
    wanLinks.push({
      dc1: d1,
      dc2: d2,
      x1: geoms[d1].cx + geoms[d1].r,
      x2: geoms[d2].cx - geoms[d2].r,
      midX: (geoms[d1].cx + geoms[d2].cx) / 2,
      cy: 220,
    });
  }
  const firstCx = geoms[dcs[0].id].cx;
  const lastCx = geoms[dcs[dcCount - 1].id].cx;
  return {
    svgWidth,
    geoms,
    wanLinks,
    wan: { x1: firstCx, x2: lastCx, midX: (firstCx + lastCx) / 2 },
  };
}

// WebAssembly Initialization Callback
window.onOps5CassandraReady = () => {
  if (state.wasmReady) return;
  console.log("==> OPS5 Cassandra Protocol Engine WebAssembly Ready");
  state.wasmReady = true;
  elements.loadingOverlay.classList.add('hidden');

  // Defer initialization to next macrotask to ensure Go main() is settled on channel
  setTimeout(() => {
    initCluster();

    // Automatically start live background cluster traffic across active datacenters
    setTimeout(() => {
      startAutoTraffic();
    }, 1000);
  }, 20);
};

async function initWasm() {
  const go = new Go();
  const wasmUrl = `main.wasm?v=${Date.now()}`;
  try {
    let result;
    if (WebAssembly.instantiateStreaming) {
      result = await WebAssembly.instantiateStreaming(
        fetch(wasmUrl, { cache: 'no-cache' }),
        go.importObject
      );
    } else {
      const resp = await fetch(wasmUrl, { cache: 'no-cache' });
      const bytes = await resp.arrayBuffer();
      result = await WebAssembly.instantiate(bytes, go.importObject);
    }
    go.run(result.instance);
    waitForWasmReady();
  } catch (err) {
    console.error("Failed to load Wasm binary:", err);
    elements.loadingOverlay.querySelector('.loading-text').innerText = "Failed to load WebAssembly";
    elements.loadingOverlay.querySelector('.loading-subtext').innerText = err.message;
  }
}

function waitForWasmReady() {
  if (typeof window.cassandraInit === 'function') {
    if (!state.wasmReady && typeof window.onOps5CassandraReady === 'function') {
      window.onOps5CassandraReady();
    }
  } else {
    setTimeout(waitForWasmReady, 50);
  }
}

// Cluster State Management
function initCluster() {
  if (!state.wasmReady) return;
  const res = window.cassandraInit();
  if (res && res.success) {
    applyClusterState(res);
    logToTerminal(`// Cluster initialized successfully with ${state.dcCount} DCs and ${state.dcCount * 6} nodes.`, "text-info");
  }
}

function applyClusterState(res) {
  if (res.cluster) state.cluster = res.cluster;
  if (res.clientDc !== undefined) state.clientDC = res.clientDc;
  if (res.coordinatorId !== undefined) state.coordinatorID = res.coordinatorId;
  if (res.wanConnected !== undefined) {
    state.wanConnected = res.wanConnected;
  }
  if (res.wanLinks !== undefined) {
    state.wanLinks = res.wanLinks;
  }
  if (res.dcCount !== undefined) {
    state.dcCount = res.dcCount;
  } else if (state.cluster && state.cluster.dcs) {
    state.dcCount = state.cluster.dcs.length;
  }
  if (res.replicasPerDc !== undefined) {
    state.replicasPerDc = res.replicasPerDc;
  } else if (state.cluster && state.cluster.replicasPerDc !== undefined) {
    state.replicasPerDc = state.cluster.replicasPerDc;
  }

  updateControlsUI();
  renderRings();
  renderWANLinkUI();
}

function renderWANLinkUI() {
  const dcCount = (state.dcCount !== undefined) ? state.dcCount : (state.cluster && state.cluster.dcs ? state.cluster.dcs.length : 0);
  if (!elements.wanLinkLayer) return;

  if (dcCount < 2 || !currentLayout || !currentLayout.wanLinks || currentLayout.wanLinks.length === 0) {
    elements.wanLinkLayer.innerHTML = '';
    elements.wanLinkLayer.style.display = 'none';
    if (elements.legendWanBadge) elements.legendWanBadge.style.display = 'none';
    return;
  }

  elements.wanLinkLayer.style.display = '';
  elements.wanLinkLayer.innerHTML = '';

  currentLayout.wanLinks.forEach((link) => {
    const linkObj = (state.wanLinks || []).find(l => (l.dc1 === link.dc1 && l.dc2 === link.dc2) || (l.dc1 === link.dc2 && l.dc2 === link.dc1));
    const isConnected = linkObj ? linkObj.connected : true;

    const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    g.setAttribute('class', `wan-btn-group ${isConnected ? '' : 'severed'}`);
    g.setAttribute('cursor', 'pointer');
    g.setAttribute('role', 'button');
    g.setAttribute('tabindex', '0');
    g.setAttribute('aria-label', `Toggle WAN Link ${link.dc1.toUpperCase()} to ${link.dc2.toUpperCase()}`);
    g.dataset.dc1 = link.dc1;
    g.dataset.dc2 = link.dc2;

    const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
    line.setAttribute('x1', link.x1);
    line.setAttribute('y1', link.cy);
    line.setAttribute('x2', link.x2);
    line.setAttribute('y2', link.cy);
    line.setAttribute('class', 'wan-line');
    g.appendChild(line);

    const pillW = (dcCount === 2) ? 140 : 116;
    const pillH = 38;
    const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
    rect.setAttribute('x', link.midX - pillW / 2);
    rect.setAttribute('y', link.cy - pillH / 2);
    rect.setAttribute('width', pillW);
    rect.setAttribute('height', pillH);
    rect.setAttribute('rx', 8);
    rect.setAttribute('class', 'wan-pill-bg');
    g.appendChild(rect);

    const d1Name = link.dc1.toUpperCase();
    const d2Name = link.dc2.toUpperCase();

    const textTitle = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    textTitle.setAttribute('x', link.midX);
    textTitle.setAttribute('y', link.cy - 3);
    textTitle.setAttribute('text-anchor', 'middle');
    textTitle.setAttribute('class', 'wan-title-text');
    textTitle.textContent = isConnected ? `⚡ WAN ${d1Name}–${d2Name}` : `✂️ ${d1Name}–${d2Name}`;
    g.appendChild(textTitle);

    const textSub = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    textSub.setAttribute('x', link.midX);
    textSub.setAttribute('y', link.cy + 11);
    textSub.setAttribute('text-anchor', 'middle');
    textSub.setAttribute('class', 'wan-sub-text');
    textSub.textContent = isConnected ? 'CONNECTED' : 'SEVERED';
    g.appendChild(textSub);

    g.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleSpecificWANLink(link.dc1, link.dc2);
    });

    elements.wanLinkLayer.appendChild(g);
  });

  // Update Legend badge status
  if (elements.legendWanBadge) {
    elements.legendWanBadge.style.display = '';
    const severedCount = (state.wanLinks || []).filter(l => !l.connected).length;
    const isPartitioned = (severedCount > 0);
    elements.legendWanBadge.classList.toggle('severed', isPartitioned);
    elements.legendWanBadge.textContent = isPartitioned
      ? `✂️ WAN: PARTITIONED (${severedCount} SEVERED)`
      : '⚡ WAN: CONNECTED';
  }
}

function toggleSpecificWANLink(dc1, dc2) {
  if (!state.wasmReady) return;
  const res = window.cassandraToggleWANLink ? window.cassandraToggleWANLink(dc1, dc2) : window.cassandraToggleWAN();
  applyClusterState(res);
  const linkObj = (state.wanLinks || []).find(l => (l.dc1 === dc1 && l.dc2 === dc2) || (l.dc1 === dc2 && l.dc2 === dc1));
  const isConnected = linkObj ? linkObj.connected : true;
  const d1Upper = dc1.toUpperCase();
  const d2Upper = dc2.toUpperCase();
  if (isConnected) {
    logToTerminal(`[WAN CONNECTIVITY] WAN link between ${d1Upper} and ${d2Upper} RESTORED. Cross-DC traffic operational.`, "text-success");
    showToast(`WAN Link ${d1Upper}–${d2Upper} Restored`);
  } else {
    logToTerminal(`[WAN PARTITION] WAN link between ${d1Upper} and ${d2Upper} SEVERED! Network partition created.`, "text-danger");
    showToast(`WAN Link ${d1Upper}–${d2Upper} Severed (Partition)`, "warn");
  }
}

function toggleWANConnection() {
  if (!state.wasmReady) return;
  const res = window.cassandraToggleWAN();
  applyClusterState(res);
  const isConnected = state.wanConnected;
  if (isConnected) {
    logToTerminal("[WAN LINK] All inter-datacenter WAN links RESTORED. Cross-DC replication operational.", "text-success");
    showToast("All WAN Links Restored (Connected)");
  } else {
    logToTerminal("[WAN PARTITION] All inter-datacenter WAN links SEVERED! Full cluster partition active.", "text-danger");
    showToast("All WAN Links Severed (Cluster Partitioned)", "warn");
  }
}

function updateControlsUI() {
  const dcCount = (state.dcCount !== undefined) ? state.dcCount : (state.cluster && state.cluster.dcs ? state.cluster.dcs.length : 2);
  const repsPerDc = (state.replicasPerDc !== undefined) ? state.replicasPerDc : 3;

  // Update Stepper Badges & Disabled states
  if (elements.dcCountBadge) elements.dcCountBadge.textContent = `${dcCount} DCs`;
  if (elements.btnRemoveDC) elements.btnRemoveDC.disabled = (dcCount <= 0);
  if (elements.btnAddDC) elements.btnAddDC.disabled = (dcCount >= 5);

  if (elements.replicaCountBadge) elements.replicaCountBadge.textContent = `${repsPerDc} / DC`;
  if (elements.btnRemoveReplica) elements.btnRemoveReplica.disabled = (repsPerDc <= 0);
  if (elements.btnAddReplica) elements.btnAddReplica.disabled = (repsPerDc >= 6);

  // Dynamically populate DC segmented toggle buttons for all active DCs
  if (elements.dcToggleGroup) {
    elements.dcToggleGroup.innerHTML = '';
    if (dcCount === 0 || !state.cluster || !state.cluster.dcs || state.cluster.dcs.length === 0) {
      const emptySpan = document.createElement('span');
      emptySpan.style.cssText = 'color:#64748b;font-size:0.8rem;padding:6px 10px;';
      emptySpan.textContent = 'No DCs';
      elements.dcToggleGroup.appendChild(emptySpan);
    } else {
      state.cluster.dcs.forEach(dc => {
        const b = document.createElement('button');
        b.className = `btn btn-segmented ${dc.id === state.clientDC ? 'active' : ''}`;
        b.dataset.dc = dc.id;
        b.innerText = `${dc.id.toUpperCase()} (${getDCRegion(dc.id)})`;
        b.addEventListener('click', () => {
          const res = window.cassandraSetClientDC(dc.id);
          applyClusterState(res);
          logToTerminal(`[CLIENT] Connected client to Datacenter ${getDCLabel(dc.id)}`, "text-info");
        });
        elements.dcToggleGroup.appendChild(b);
      });
    }
  }

  // Populate coordinator dropdown
  elements.selectCoordinator.innerHTML = '';
  if (dcCount === 0 || !state.cluster || !state.cluster.dcs || state.cluster.dcs.length === 0) {
    const opt = document.createElement('option');
    opt.value = '';
    opt.innerText = 'None (0 Datacenters)';
    opt.disabled = true;
    opt.selected = true;
    elements.selectCoordinator.appendChild(opt);
    elements.selectCoordinator.disabled = true;
    return;
  }

  elements.selectCoordinator.disabled = false;
  if (!state.cluster.dcs.some(d => d.id === state.clientDC)) {
    state.clientDC = state.cluster.dcs[0].id;
  }

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

// Render SVG Ring Topology for 0 to 5 Datacenters
function renderRings() {
  elements.nodesLayer.innerHTML = '';
  const dcCount = (state.dcCount !== undefined) ? state.dcCount : (state.cluster && state.cluster.dcs ? state.cluster.dcs.length : 2);
  currentLayout = computeLayout(state.cluster ? state.cluster.dcs : []);

  // Update SVG viewBox and horizontal min-width for responsive or scrolling display
  elements.ringsSvg.setAttribute('viewBox', `0 0 ${currentLayout.svgWidth} 500`);
  elements.ringsSvg.style.minWidth = (currentLayout.svgWidth > 1100) ? `${currentLayout.svgWidth}px` : '100%';

  const ringsBaseLayer = document.getElementById('rings-base-layer');
  if (ringsBaseLayer) {
    ringsBaseLayer.innerHTML = '';
  }

  if (dcCount === 0 || !state.cluster || !state.cluster.dcs || state.cluster.dcs.length === 0) {
    if (elements.clusterEmptyMessage && ringsBaseLayer) {
      elements.clusterEmptyMessage.style.display = '';
      const rect = elements.clusterEmptyMessage.querySelector('rect');
      const texts = elements.clusterEmptyMessage.querySelectorAll('text');
      const msgX = (currentLayout.svgWidth - 500) / 2;
      if (rect) rect.setAttribute('x', msgX);
      if (texts[0]) texts[0].setAttribute('x', currentLayout.svgWidth / 2);
      if (texts[1]) texts[1].setAttribute('x', currentLayout.svgWidth / 2);
      ringsBaseLayer.appendChild(elements.clusterEmptyMessage);
    }
    if (elements.wanLinkLayer) elements.wanLinkLayer.style.display = 'none';
    if (elements.legendWanBadge) elements.legendWanBadge.style.display = 'none';
    if (elements.clientLayer) elements.clientLayer.innerHTML = '';
    return;
  }

  if (elements.clusterEmptyMessage) {
    elements.clusterEmptyMessage.style.display = 'none';
  }

  // Dynamic WAN Link layer: rendered per adjacent pair
  renderWANLinkUI();

  // Draw ring bases dynamically
  if (ringsBaseLayer) {
    state.cluster.dcs.forEach((dc, idx) => {
      const geom = currentLayout.geoms[dc.id];
      if (!geom) return;

      const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', `ring-base-${dc.id}`);

      const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      circle.setAttribute('cx', geom.cx);
      circle.setAttribute('cy', geom.cy);
      circle.setAttribute('r', geom.r);
      circle.setAttribute('class', 'ring-circle');
      g.appendChild(circle);

      const title = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      title.setAttribute('x', geom.cx);
      title.setAttribute('y', geom.cy - 5);
      title.setAttribute('text-anchor', 'middle');
      title.setAttribute('class', 'dc-title');
      title.textContent = `Datacenter ${idx + 1}`;
      g.appendChild(title);

      const subtitle = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      subtitle.setAttribute('x', geom.cx);
      subtitle.setAttribute('y', geom.cy + 15);
      subtitle.setAttribute('text-anchor', 'middle');
      subtitle.setAttribute('class', 'dc-subtitle');
      subtitle.textContent = `${getDCRegion(dc.id)} (${dc.nodes.length} Nodes)`;
      g.appendChild(subtitle);

      ringsBaseLayer.appendChild(g);
    });
  }

  // Draw nodes across all active datacenters
  state.cluster.dcs.forEach(dc => {
    const geom = currentLayout.geoms[dc.id];
    if (!geom) return;

    dc.nodes.forEach((n, idx) => {
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

  const dcCount = (state.dcCount !== undefined) ? state.dcCount : (state.cluster && state.cluster.dcs ? state.cluster.dcs.length : 2);
  if (dcCount === 0 || !state.cluster || !state.cluster.dcs || state.cluster.dcs.length === 0 || !currentLayout) {
    return;
  }

  const dcId = state.cluster.dcs.some(d => d.id === state.clientDC) ? state.clientDC : state.cluster.dcs[0].id;
  const geom = currentLayout.geoms[dcId] || { cx: 400, cy: 220, r: 140 };
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
  text.textContent = `💻 Client → ${getDCLabel(dcId)}`;
  g.appendChild(text);

  clientLayer.appendChild(g);
}

function getClientCoords() {
  if (!currentLayout) return { x: 400, y: 460 - 18 };
  const geom = currentLayout.geoms[state.clientDC] || currentLayout.geoms['dc1'] || { cx: 400 };
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

  // Invisible extended hit-target for effortless and reliable selection
  const hitCircle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  hitCircle.setAttribute('cx', x);
  hitCircle.setAttribute('cy', y);
  hitCircle.setAttribute('r', NODE_RADIUS + 8);
  hitCircle.setAttribute('fill', 'transparent');
  hitCircle.setAttribute('class', 'node-hitbox');
  g.appendChild(hitCircle);

  // Node Circle
  const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  circle.setAttribute('cx', x);
  circle.setAttribute('cy', y);
  circle.setAttribute('r', NODE_RADIUS);
  circle.setAttribute('class', 'node-circle');
  if (node.isReplica) {
    circle.setAttribute('filter', 'url(#glow-replica)');
  }
  g.appendChild(circle);

  // Node ID label (e.g. dc1-n1 -> N1, dc3-n4 -> N4)
  const textId = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  textId.setAttribute('x', x);
  textId.setAttribute('y', isClientCoord ? y - 4 : y - 6);
  textId.setAttribute('text-anchor', 'middle');
  textId.setAttribute('class', 'node-label');
  textId.textContent = node.id.replace(/^dc\d+-/, 'N');
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
      <div><strong>Datacenter:</strong> <code>${getDCLabel(node.dc)}</code></div>
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

// Stepper Handlers: Datacenters (+/-)
if (elements.btnAddDC) {
  elements.btnAddDC.onclick = () => {
    if (!state.wasmReady) return;
    const res = window.cassandraAddDC();
    if (res && res.success) {
      applyClusterState(res);
      resetMetrics();
      logToTerminal(`[TOPOLOGY] Added Datacenter. Total DCs: ${state.dcCount}`, "text-info");
      showToast(`Added Datacenter (${state.dcCount} DCs)`);
    } else if (res && res.error) {
      showToast(res.error);
    }
  };
}

if (elements.btnRemoveDC) {
  elements.btnRemoveDC.onclick = () => {
    if (!state.wasmReady) return;
    const res = window.cassandraRemoveDC();
    if (res && res.success) {
      applyClusterState(res);
      resetMetrics();
      logToTerminal(`[TOPOLOGY] Removed Datacenter. Total DCs: ${state.dcCount}`, "text-warn");
      showToast(`Removed Datacenter (${state.dcCount} DCs remaining)`);
    } else if (res && res.error) {
      showToast(res.error);
    }
  };
}

// Stepper Handlers: Replicas per DC (+/-)
if (elements.btnAddReplica) {
  elements.btnAddReplica.onclick = () => {
    if (!state.wasmReady) return;
    const res = window.cassandraAddReplica();
    if (res && res.success) {
      applyClusterState(res);
      resetMetrics();
      logToTerminal(`[REPLICATION] Added replica per DC. Current: ${state.replicasPerDc} replicas/DC`, "text-info");
      showToast(`Replicas per DC: ${state.replicasPerDc}`);
    } else if (res && res.error) {
      showToast(res.error);
    }
  };
}

if (elements.btnRemoveReplica) {
  elements.btnRemoveReplica.onclick = () => {
    if (!state.wasmReady) return;
    const res = window.cassandraRemoveReplica();
    if (res && res.success) {
      applyClusterState(res);
      resetMetrics();
      logToTerminal(`[REPLICATION] Removed replica per DC. Current: ${state.replicasPerDc} replicas/DC`, "text-warn");
      showToast(`Replicas per DC: ${state.replicasPerDc}`);
    } else if (res && res.error) {
      showToast(res.error);
    }
  };
}

// Reset Cluster Handler
elements.btnResetCluster.onclick = () => {
  const res = window.cassandraReset();
  applyClusterState(res);
  resetMetrics();
  logToTerminal("[CLUSTER] Cluster reset to default pristine state (all 12 nodes UN).", "text-info");
  showToast("Cluster reset to default state");
};

// WAN Link Toggle Handlers (Legend badge toggles all links)
if (elements.legendWanBadge) {
  elements.legendWanBadge.onclick = toggleWANConnection;
}

function resetMetrics() {
  const repsPerDc = (state.replicasPerDc !== undefined) ? state.replicasPerDc : 3;
  const dcCount = (state.dcCount !== undefined) ? state.dcCount : 2;
  const reqLocal = repsPerDc > 0 ? Math.floor(repsPerDc / 2) + 1 : 1;
  const reqTotal = getRequiredAcks(state.consistencyLevel, state.clientDC);

  elements.resultStatusBadge.className = 'badge badge-success';
  elements.resultStatusBadge.innerText = 'Ready';
  elements.statLocalAcks.innerText = `0 / ${reqLocal}`;
  elements.statTotalAcks.innerText = `0 / ${reqTotal}`;
  elements.statLocalAlive.innerText = `${repsPerDc} / ${repsPerDc}`;
  elements.statTotalAlive.innerText = `${dcCount * repsPerDc} / ${dcCount * repsPerDc}`;
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
    const activeDCs = (state.cluster && state.cluster.dcs) ? state.cluster.dcs.map(d => d.id) : [];
    if (activeDCs.length === 0) {
      elements.trafficStatusText.innerHTML = `Live: <strong>Cluster Offline (0 DCs)</strong>`;
      return;
    }
    const remaining = Math.max(0, (trafficState.nextTickTime - Date.now()) / 1000);
    const target = activeDCs.includes(trafficState.nextDC) ? trafficState.nextDC : activeDCs[0];
    elements.trafficStatusText.innerHTML = `Live: <strong>Active (~5s)</strong> &bull; Next: <strong>${target.toUpperCase()}</strong> in ${remaining.toFixed(1)}s`;
  }, 200);
}

async function executeAutoTrafficTick() {
  if (!trafficState.isRunning) return;

  if (state.isExecuting) {
    // If a manual client query is currently running, retry shortly
    scheduleNextTraffic(800);
    return;
  }

  const activeDCs = (state.cluster && state.cluster.dcs) ? state.cluster.dcs.map(d => d.id) : [];
  if (activeDCs.length === 0) {
    scheduleNextTraffic(2000);
    return;
  }

  // 1. Pick DC from active datacenters (cycle through all active DCs)
  let targetDC = trafficState.nextDC;
  let idx = activeDCs.indexOf(targetDC);
  if (idx === -1) {
    idx = 0;
    targetDC = activeDCs[0];
  }
  const nextIdx = (idx + 1) % activeDCs.length;
  trafficState.nextDC = activeDCs[nextIdx];

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

    const dcLabel = getDCLabel(targetDC);
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

function getNodeDC(nodeID) {
  if (!nodeID || !state.cluster || !state.cluster.dcs) return null;
  for (const dc of state.cluster.dcs) {
    if (dc.nodes && dc.nodes.some(n => n.id === nodeID)) return dc.id;
  }
  return null;
}

function getSeveredWANBoundary(fromDC, toDC) {
  if (!state.cluster || !state.cluster.dcs || !currentLayout || !currentLayout.wanLinks) {
    return { x: 550, y: 220 };
  }
  const dcs = state.cluster.dcs;
  const idxFrom = dcs.findIndex(d => d.id === fromDC);
  const idxTo = dcs.findIndex(d => d.id === toDC);
  if (idxFrom === -1 || idxTo === -1) {
    const fallbackMidX = (currentLayout && currentLayout.wan) ? currentLayout.wan.midX : 550;
    return { x: fallbackMidX, y: 220 };
  }

  const indices = [];
  if (idxFrom < idxTo) {
    for (let i = idxFrom; i < idxTo; i++) indices.push(i);
  } else {
    for (let i = idxFrom - 1; i >= idxTo; i--) indices.push(i);
  }

  for (const i of indices) {
    if (i < 0 || i >= dcs.length - 1) continue;
    const d1 = dcs[i].id;
    const d2 = dcs[i + 1].id;
    const linkObj = (state.wanLinks || []).find(l => (l.dc1 === d1 && l.dc2 === d2) || (l.dc1 === d2 && l.dc2 === d1));
    const isConnected = linkObj ? linkObj.connected : true;
    if (!isConnected) {
      const layoutLink = currentLayout.wanLinks.find(l => (l.dc1 === d1 && l.dc2 === d2) || (l.dc1 === d2 && l.dc2 === d1));
      if (layoutLink) {
        return { x: layoutLink.midX, y: 220 };
      }
    }
  }

  const fallbackMidX = (currentLayout && currentLayout.wan) ? currentLayout.wan.midX : 550;
  return { x: fallbackMidX, y: 220 };
}

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
  const coordDC = getNodeDC(res.coordinator) || state.clientDC || 'dc1';
  res.messages.forEach(m => {
    if (m.kind === 'request') {
      const destCoords = getNodeCoords(m.toNode);
      if (destCoords) {
        const isSevered = (m.status === 'dropped' && m.isLocal === false);
        if (isSevered) {
          const destDC = getNodeDC(m.toNode);
          const wanBoundaryCoords = getSeveredWANBoundary(coordDC, destDC);
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
  if (!state.cluster || !state.cluster.dcs) return null;
  if (!currentLayout) currentLayout = computeLayout(state.cluster.dcs);
  for (const dc of state.cluster.dcs) {
    const geom = currentLayout.geoms ? currentLayout.geoms[dc.id] : null;
    if (!geom) continue;
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
  const repsPerDc = (state.replicasPerDc !== undefined) ? state.replicasPerDc : (res.replicasPerDc !== undefined ? res.replicasPerDc : 3);
  const dcCount = (state.dcCount !== undefined) ? state.dcCount : (res.dcCount !== undefined ? res.dcCount : 2);
  const totalReps = dcCount * repsPerDc;
  const reqTotal = getRequiredAcks(res.consistencyLevel, res.localDc);
  const reqLocal = repsPerDc > 0 ? Math.floor(repsPerDc / 2) + 1 : 1;

  elements.statCL.innerText = res.consistencyLevel;
  const localAcks = res.ackResult ? res.ackResult.localAcks : 0;
  const totalAcks = res.ackResult ? res.ackResult.totalAcks : 0;
  elements.statLocalAcks.innerText = `${localAcks} / ${reqLocal}`;
  elements.statTotalAcks.innerText = `${totalAcks} / ${reqTotal}`;
  const localAlive = res.liveTally ? res.liveTally.localAlive : 0;
  const totalAlive = res.liveTally ? res.liveTally.totalAlive : 0;
  elements.statLocalAlive.innerText = `${localAlive} / ${repsPerDc}`;
  elements.statTotalAlive.innerText = `${totalAlive} / ${totalReps}`;
  elements.statCycles.innerText = res.stats ? res.stats.cycleCount : 0;

  // Quorum Progress Bar
  const achieved = (res.consistencyLevel === 'LOCAL_QUORUM') ? localAcks : totalAcks;
  const pct = (reqTotal > 0) ? Math.min(Math.round((achieved / reqTotal) * 100), 100) : 0;
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
  const localDcName = res.localDc ? res.localDc.toUpperCase() : 'NONE';
  const remoteReps = Math.max(0, totalReps - repsPerDc);
  logToTerminal(`[DISPATCH] Natural endpoints: ${totalReps} replicas (${repsPerDc} local in ${localDcName}, ${remoteReps} remote)`, "text-info");
  if (res.liveTally) {
    const remoteAlive = res.liveTally.remoteAlive || 0;
    logToTerminal(`[PRE-FLIGHT] Live replicas tally: Local=${localAlive}/${repsPerDc}, Remote=${remoteAlive}/${remoteReps}, Total=${totalAlive}/${totalReps}`, "text-info");
  }

  if (!res.success) {
    logToTerminal(`[EXCEPTION] ${res.errorReason}`, "text-danger");
    logToTerminal(`[RESULT] Query FAILED in ${res.stats ? res.stats.elapsedMs.toFixed(2) : '0.00'}ms (${res.stats ? res.stats.cycleCount : 0} OPS5 cycles)`, "text-danger");
    if (origin === 'client') showToast(`Query Failed: ${res.errorReason}`);
  } else {
    logToTerminal(`[ACKS] Coordinator received ${totalAcks} total acks (${localAcks} local in ${localDcName})`, "text-success");
    if (res.queryType === 'read') {
      logToTerminal(`[VALUE RESOLUTION] Read resolved value="${res.resolvedValue}" (Timestamp=${res.highestTimestamp})`, "text-success");
    }
    logToTerminal(`[RESULT] Query SUCCESS at CL=${res.consistencyLevel} in ${res.stats ? res.stats.elapsedMs.toFixed(2) : '0.00'}ms (${res.stats ? res.stats.cycleCount : 0} OPS5 cycles)`, "text-success");
    if (origin === 'client') showToast(`Query Success at ${res.consistencyLevel}!`);
  }
}

function getRequiredAcks(cl, localDc) {
  const repsPerDc = (state.replicasPerDc !== undefined) ? state.replicasPerDc : 3;
  const dcCount = (state.dcCount !== undefined) ? state.dcCount : 2;
  const totalReplicas = dcCount * repsPerDc;

  switch (cl) {
    case 'ONE': return 1;
    case 'TWO': return 2;
    case 'THREE': return 3;
    case 'LOCAL_QUORUM': return repsPerDc > 0 ? Math.floor(repsPerDc / 2) + 1 : 1;
    case 'QUORUM': return totalReplicas > 0 ? Math.floor(totalReplicas / 2) + 1 : 1;
    default: return 1;
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
