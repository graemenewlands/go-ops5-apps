// State management
const state = {
  wasmReady: false,
  schema: null,
  selectedFields: new Map(), // key: "Table.Column" -> { table, column, type }
  tablePositions: new Map(), // key: "Table" -> { x, y }
  pan: { x: 30, y: 20 },
  zoom: 0.82,
  isDraggingCanvas: false,
  isDraggingTable: null,
  dragStart: { x: 0, y: 0 },
  tableDragOffset: { x: 0, y: 0 },
  lastResult: null,
};

// DOM references
const elements = {
  loadingOverlay: document.getElementById('loading-overlay'),
  selectSchema: document.getElementById('select-schema'),
  presetsContainer: document.getElementById('presets-container'),
  selectedFieldsTray: document.getElementById('selected-fields-tray'),
  selectedFieldsList: document.getElementById('selected-fields-list'),
  trayCount: document.getElementById('tray-count'),
  diagramContainer: document.getElementById('diagram-container'),
  diagramSvg: document.getElementById('diagram-svg'),
  transformGroup: document.getElementById('diagram-transform-group'),
  linksLayer: document.getElementById('diagram-links-layer'),
  tablesLayer: document.getElementById('diagram-tables-layer'),
  btnZoomIn: document.getElementById('btn-zoom-in'),
  btnZoomOut: document.getElementById('btn-zoom-out'),
  btnZoomFit: document.getElementById('btn-zoom-fit'),
  inputFieldSearch: document.getElementById('input-field-search'),
  selectionCounter: document.getElementById('selection-counter'),
  btnClearSelection: document.getElementById('btn-clear-selection'),
  btnGenerateView: document.getElementById('btn-generate-view'),
  viewSection: document.getElementById('view-section'),
  viewEmptyState: document.getElementById('view-empty-state'),
  viewErrorState: document.getElementById('view-error-state'),
  viewErrorMessage: document.getElementById('view-error-message'),
  viewResultContent: document.getElementById('view-result-content'),
  statCycles: document.getElementById('stat-cycles'),
  statTime: document.getElementById('stat-time'),
  statWmes: document.getElementById('stat-wmes'),
  statJoins: document.getElementById('stat-joins'),
  statRoot: document.getElementById('stat-root'),
  joinPathContainer: document.getElementById('join-path-container'),
  schemaColumnsTbody: document.getElementById('schema-columns-tbody'),
  colCountBadge: document.getElementById('col-count-badge'),
  sqlCodeBlock: document.getElementById('sql-code-block'),
  btnCopySql: document.getElementById('btn-copy-sql'),
  previewDataTable: document.getElementById('preview-data-table'),
  previewDataThead: document.getElementById('preview-data-thead'),
  previewDataTbody: document.getElementById('preview-data-tbody'),
  rowCountBadge: document.getElementById('row-count-badge'),
  btnToggleInspector: document.getElementById('btn-toggle-inspector'),
  ruleDrawer: document.getElementById('rule-drawer'),
  drawerBackdrop: document.getElementById('drawer-backdrop'),
  btnCloseDrawer: document.getElementById('btn-close-drawer'),
  ruleCodeBlock: document.getElementById('rule-code-block'),
  toast: document.getElementById('toast'),
};

// WebAssembly Initialization
window.onOps5SchemaReady = () => {
  console.log("==> OPS5 Schema Engine WebAssembly Module Ready");
  state.wasmReady = true;
  elements.loadingOverlay.classList.add('hidden');
  initSchema("chinook");
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

// Load and Render Schema
function initSchema(schemaID) {
  if (!state.wasmReady) return;

  const res = window.schemaInit(schemaID);
  if (!res || !res.success) {
    showToast("Error initializing schema: " + (res ? res.error : "unknown"));
    return;
  }

  loadSchemaDefinition();
}

function loadSchemaDefinition() {
  const defRes = window.schemaGetDefinition();
  if (!defRes || !defRes.success) {
    showToast("Failed to retrieve schema definition");
    return;
  }

  state.schema = defRes.schema;
  state.selectedFields.clear();
  state.tablePositions.clear();

  // Initialize table positions
  state.schema.tables.forEach(t => {
    state.tablePositions.set(t.name, { x: t.x || 50, y: t.y || 50 });
  });

  renderPresets();
  renderDiagram();
  updateSelectionUI();
  if (typeof window.schemaClear === 'function') {
    window.schemaClear();
  }
  resetMaterializedViewSection();
}

// Preset selections
function renderPresets() {
  elements.presetsContainer.innerHTML = '';
  if (!state.schema.presets) return;

  state.schema.presets.forEach(p => {
    const chip = document.createElement('button');
    chip.className = 'preset-chip';
    chip.innerText = p.title;
    chip.title = p.description;
    chip.onclick = () => applyPreset(p);
    elements.presetsContainer.appendChild(chip);
  });
}

function applyPreset(preset) {
  state.selectedFields.clear();
  preset.fields.forEach(([tbl, col]) => {
    state.selectedFields.set(`${tbl}.${col}`, { table: tbl, column: col });
  });

  // Highlight active preset chip
  document.querySelectorAll('.preset-chip').forEach(c => {
    c.classList.toggle('active', c.innerText === preset.title);
  });

  updateTableSelectionsInDOM();
  updateSelectionUI();
  showToast(`Applied preset: ${preset.title}`);

  // Automatically trigger view synthesis
  generateMaterializedView();
}

// Diagram Rendering & Interaction
function renderDiagram() {
  elements.linksLayer.innerHTML = '';
  elements.tablesLayer.innerHTML = '';

  const tableMap = new Map();
  state.schema.tables.forEach(t => tableMap.set(t.name, t));

  // Render SVG connector paths for relations
  renderRelationshipLinks();

  // Render tables
  state.schema.tables.forEach(t => {
    const pos = state.tablePositions.get(t.name) || { x: 50, y: 50 };
    renderTableNode(t, pos);
  });

  updateTransform();
}

function renderRelationshipLinks() {
  elements.linksLayer.innerHTML = '';
  if (!state.schema.relations) return;

  const isMNJunction = (tblName) => {
    return state.schema.mnRelations && state.schema.mnRelations.some(mn => mn.junctionTable === tblName);
  };

  state.schema.relations.forEach(r => {
    const fromPos = state.tablePositions.get(r.fromTable);
    const toPos = state.tablePositions.get(r.toTable);
    if (!fromPos || !toPos) return;

    const fromSelected = hasSelectedFieldInTable(r.fromTable);
    const toSelected = hasSelectedFieldInTable(r.toTable);
    const isActive = fromSelected && toSelected;
    const isMN = isMNJunction(r.fromTable) || isMNJunction(r.toTable);

    // Approximate link attachment coordinates
    const p1 = { x: fromPos.x + 120, y: fromPos.y + 40 };
    const p2 = { x: toPos.x + 120, y: toPos.y + 40 };

    const dx = p2.x - p1.x;
    const dy = p2.y - p1.y;
    const cx1 = p1.x + dx * 0.5;
    const cy1 = p1.y;
    const cx2 = p1.x + dx * 0.5;
    const cy2 = p2.y;

    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    const d = `M ${p1.x} ${p1.y} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${p2.x} ${p2.y}`;
    path.setAttribute('d', d);
    path.setAttribute('class', `link-path ${isActive ? 'active' : ''} ${isMN ? 'mn-link' : ''}`);
    path.setAttribute('id', `link-${r.fromTable}-${r.toTable}`);

    elements.linksLayer.appendChild(path);
  });
}

function hasSelectedFieldInTable(tableName) {
  for (const [key, val] of state.selectedFields.entries()) {
    if (val.table === tableName) return true;
  }
  return false;
}

function renderTableNode(table, pos) {
  const isJunction = state.schema.mnRelations && state.schema.mnRelations.some(mn => mn.junctionTable === table.name);
  const width = 230;
  const headerHeight = 38;
  const rowHeight = 28;
  const totalHeight = headerHeight + (table.columns.length * rowHeight) + 12;

  const fo = document.createElementNS('http://www.w3.org/2000/svg', 'foreignObject');
  fo.setAttribute('x', pos.x);
  fo.setAttribute('y', pos.y);
  fo.setAttribute('width', width);
  fo.setAttribute('height', totalHeight);
  fo.setAttribute('id', `table-node-${table.name}`);

  const card = document.createElement('div');
  card.className = `table-node ${isJunction ? 'junction-table' : ''}`;
  card.id = `table-card-${table.name}`;

  // Header (draggable)
  const header = document.createElement('div');
  header.className = 'table-header';
  header.innerHTML = `
    <div class="table-title">
      <span>${table.name}</span>
      <span class="table-selected-badge" id="table-badge-${table.name}" style="display:none;">0 sel</span>
    </div>
    <span class="table-tag ${isJunction ? 'table-tag-mn' : ''}">${isJunction ? 'M-N' : 'Table'}</span>
  `;

  // Drag table handling
  header.addEventListener('mousedown', (e) => {
    e.stopPropagation();
    state.isDraggingTable = table.name;
    state.dragStart = { x: e.clientX, y: e.clientY };
    const curPos = state.tablePositions.get(table.name);
    state.tableDragOffset = { x: curPos.x, y: curPos.y };
  });

  card.appendChild(header);

  // Column rows
  const colContainer = document.createElement('div');
  colContainer.className = 'table-columns';

  table.columns.forEach(col => {
    const isSelected = state.selectedFields.has(`${table.name}.${col.name}`);
    const row = document.createElement('div');
    row.className = `col-row ${isSelected ? 'selected' : ''}`;
    row.id = `col-${table.name}-${col.name}`;

    let keyIcon = '';
    if (col.isPk) {
      keyIcon = '<span class="col-key-icon col-key-pk" title="Primary Key">🔑</span>';
    } else if (col.isFk) {
      keyIcon = `<span class="col-key-icon col-key-fk" title="Foreign Key -> ${col.fkTable}.${col.fkColumn}">🔗</span>`;
    } else {
      keyIcon = '<span class="col-key-icon">&bull;</span>';
    }

    row.innerHTML = `
      <div class="col-left">
        <input type="checkbox" class="col-checkbox" ${isSelected ? 'checked' : ''} />
        ${keyIcon}
        <span class="col-name">${col.name}</span>
      </div>
      <span class="col-type">${col.type}</span>
    `;

    row.addEventListener('click', (e) => {
      // Toggle selection
      toggleFieldSelection(table.name, col.name, col.type);
    });

    const checkbox = row.querySelector('.col-checkbox');
    checkbox.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleFieldSelection(table.name, col.name, col.type);
    });

    colContainer.appendChild(row);
  });

  card.appendChild(colContainer);
  fo.appendChild(card);
  elements.tablesLayer.appendChild(fo);
}

function toggleFieldSelection(tableName, colName, colType) {
  const key = `${tableName}.${colName}`;
  if (state.selectedFields.has(key)) {
    state.selectedFields.delete(key);
  } else {
    state.selectedFields.set(key, { table: tableName, column: colName, type: colType });
  }

  updateTableSelectionsInDOM();
  updateSelectionUI();
  renderRelationshipLinks();
}

function updateTableSelectionsInDOM() {
  if (!state.schema) return;

  state.schema.tables.forEach(t => {
    const card = document.getElementById(`table-card-${t.name}`);
    const badge = document.getElementById(`table-badge-${t.name}`);

    let tableSelectedCount = 0;
    t.columns.forEach(c => {
      if (state.selectedFields.has(`${t.name}.${c.name}`)) {
        tableSelectedCount++;
      }
    });

    if (card) {
      card.classList.toggle('active-table', tableSelectedCount > 0);
    }
    if (badge) {
      if (tableSelectedCount > 0) {
        badge.innerText = `${tableSelectedCount} sel`;
        badge.style.display = 'inline-block';
      } else {
        badge.style.display = 'none';
      }
    }

    t.columns.forEach(c => {
      const row = document.getElementById(`col-${t.name}-${c.name}`);
      if (row) {
        const isSelected = state.selectedFields.has(`${t.name}.${c.name}`);
        row.classList.toggle('selected', isSelected);
        const cb = row.querySelector('.col-checkbox');
        if (cb) cb.checked = isSelected;
      }
    });
  });
}

function updateSelectionUI() {
  const count = state.selectedFields.size;
  const tableSet = new Set();
  state.selectedFields.forEach(f => tableSet.add(f.table));

  elements.selectionCounter.innerText = `${count} field${count === 1 ? '' : 's'} across ${tableSet.size} table${tableSet.size === 1 ? '' : 's'}`;
  if (elements.trayCount) {
    elements.trayCount.innerText = count;
  }

  // Render dismissible pills in tray
  if (elements.selectedFieldsList) {
    elements.selectedFieldsList.innerHTML = '';
    if (count === 0) {
      elements.selectedFieldsList.innerHTML = '<span class="empty-tray-hint">Click column checkboxes or rows in table cards to add fields</span>';
    } else {
      state.selectedFields.forEach((field, key) => {
        const pill = document.createElement('div');
        pill.className = 'selected-field-pill';
        pill.innerHTML = `
          <span class="table-prefix">${field.table}.</span><span>${field.column}</span>
          <button class="remove-field-btn" title="Remove field">&times;</button>
        `;
        pill.querySelector('.remove-field-btn').onclick = (e) => {
          e.stopPropagation();
          toggleFieldSelection(field.table, field.column, field.type);
        };
        elements.selectedFieldsList.appendChild(pill);
      });
    }
  }
}

// Canvas Pan & Zoom Handlers
elements.diagramContainer.addEventListener('mousedown', (e) => {
  if (e.target.closest('.table-node')) return;
  state.isDraggingCanvas = true;
  state.dragStart = { x: e.clientX, y: e.clientY };
});

window.addEventListener('mousemove', (e) => {
  if (state.isDraggingCanvas) {
    const dx = e.clientX - state.dragStart.x;
    const dy = e.clientY - state.dragStart.y;
    state.pan.x += dx;
    state.pan.y += dy;
    state.dragStart = { x: e.clientX, y: e.clientY };
    updateTransform();
  } else if (state.isDraggingTable) {
    const dx = (e.clientX - state.dragStart.x) / state.zoom;
    const dy = (e.clientY - state.dragStart.y) / state.zoom;
    const newX = state.tableDragOffset.x + dx;
    const newY = state.tableDragOffset.y + dy;

    state.tablePositions.set(state.isDraggingTable, { x: newX, y: newY });
    const fo = document.getElementById(`table-node-${state.isDraggingTable}`);
    if (fo) {
      fo.setAttribute('x', newX);
      fo.setAttribute('y', newY);
    }
    renderRelationshipLinks();
  }
});

window.addEventListener('mouseup', () => {
  state.isDraggingCanvas = false;
  state.isDraggingTable = null;
});

elements.diagramContainer.addEventListener('wheel', (e) => {
  e.preventDefault();
  const zoomFactor = e.deltaY < 0 ? 1.08 : 0.92;
  const newZoom = Math.min(Math.max(state.zoom * zoomFactor, 0.4), 2.5);
  state.zoom = newZoom;
  updateTransform();
});

elements.btnZoomIn.onclick = () => {
  state.zoom = Math.min(state.zoom * 1.15, 2.5);
  updateTransform();
};

elements.btnZoomOut.onclick = () => {
  state.zoom = Math.max(state.zoom * 0.85, 0.4);
  updateTransform();
};

elements.btnZoomFit.onclick = () => {
  state.zoom = 0.9;
  state.pan = { x: 30, y: 30 };
  updateTransform();
};

function updateTransform() {
  elements.transformGroup.setAttribute(
    'transform',
    `translate(${state.pan.x}, ${state.pan.y}) scale(${state.zoom})`
  );
}

// Field search filter
elements.inputFieldSearch.addEventListener('input', (e) => {
  const query = e.target.value.toLowerCase().trim();
  document.querySelectorAll('.col-row').forEach(row => {
    const colName = row.querySelector('.col-name').innerText.toLowerCase();
    if (!query || colName.includes(query)) {
      row.style.display = 'flex';
    } else {
      row.style.display = 'none';
    }
  });
});

// Clear selection
elements.btnClearSelection.onclick = () => {
  state.selectedFields.clear();
  state.lastResult = null;
  if (typeof window.schemaClear === 'function') {
    window.schemaClear();
  }
  document.querySelectorAll('.preset-chip').forEach(c => c.classList.remove('active'));
  updateTableSelectionsInDOM();
  updateSelectionUI();
  renderRelationshipLinks();
  resetMaterializedViewSection();
  showToast("Cleared field selections");
};

// Materialized View Generation (Done button)
elements.btnGenerateView.onclick = () => {
  generateMaterializedView();
};

function generateMaterializedView() {
  if (state.selectedFields.size === 0) {
    showToast("Please select at least one field on the E-R diagram.");
    return;
  }

  const selectedArray = [];
  state.selectedFields.forEach(f => {
    selectedArray.push([f.table, f.column]);
  });

  const res = window.schemaGenerateView(JSON.stringify(selectedArray));
  if (!res || !res.success) {
    const errorMsg = (res && res.error) ? res.error : "Failed to synthesize materialized view: tables are not connected";
    showToast(errorMsg);
    renderMaterializedViewError(errorMsg);
    elements.viewSection.scrollIntoView({ behavior: 'smooth' });
    return;
  }

  if (elements.viewErrorState) {
    elements.viewErrorState.style.display = 'none';
  }
  state.lastResult = res;
  renderMaterializedViewResult(res);

  // Smooth scroll to view section
  elements.viewSection.scrollIntoView({ behavior: 'smooth' });
}

function renderMaterializedViewResult(res) {
  if (!res) return;

  const joins = Array.isArray(res.joins) ? res.joins : [];
  const columns = Array.isArray(res.columns) ? res.columns : [];
  const stats = res.stats || {};

  elements.viewEmptyState.style.display = 'none';
  elements.viewResultContent.style.display = 'block';

  // Stats
  elements.statCycles.innerText = stats.cycleCount ?? 0;
  elements.statTime.innerText = `${(stats.elapsedMs ?? 0).toFixed(2)} ms`;
  elements.statWmes.innerText = stats.wmeCount ?? 0;
  elements.statJoins.innerText = joins.length;
  elements.statRoot.innerText = res.rootTable || '-';

  // Join Path Chips
  elements.joinPathContainer.innerHTML = '';
  const rootChip = document.createElement('div');
  rootChip.className = 'join-chip root-chip';
  rootChip.innerHTML = `<strong>FROM</strong> <span>${res.rootTable || '-'}</span>`;
  elements.joinPathContainer.appendChild(rootChip);

  if (joins.length === 0) {
    const singleChip = document.createElement('div');
    singleChip.className = 'join-chip';
    singleChip.innerText = "Single Table (No Joins Required)";
    elements.joinPathContainer.appendChild(singleChip);
  } else {
    joins.forEach(j => {
      const arrow = document.createElement('span');
      arrow.className = 'join-arrow';
      arrow.innerText = '➔';
      elements.joinPathContainer.appendChild(arrow);

      const jChip = document.createElement('div');
      jChip.className = 'join-chip';
      jChip.innerHTML = `<strong>${j.joinType}</strong> ${j.rightTable} <span class="code-pill">${j.leftTable}.${j.leftCol} = ${j.rightTable}.${j.rightCol}</span>`;
      elements.joinPathContainer.appendChild(jChip);
    });
  }

  // Schema Columns Table
  elements.colCountBadge.innerText = `${columns.length} Columns`;
  elements.schemaColumnsTbody.innerHTML = '';

  columns.forEach(c => {
    const tr = document.createElement('tr');
    const isRenamed = c.name.toLowerCase() !== c.sourceCol.toLowerCase();
    const roleBadge = c.isPk ? '<span class="badge" style="background:#f59e0b22;color:#f59e0b;">Primary Key</span>' : '<span class="badge badge-subtle">Dimension/Measure</span>';
    const nameDisplay = isRenamed ? `<strong>${c.name}</strong> <span class="badge" style="background:#0284c722;color:#38bdf8;">Aliased</span>` : `<strong>${c.name}</strong>`;

    tr.innerHTML = `
      <td>${nameDisplay}</td>
      <td><span class="code-pill">${c.sourceTable}</span></td>
      <td><code>${c.sourceCol}</code></td>
      <td><span class="col-type">${c.type}</span></td>
      <td>${roleBadge}</td>
    `;
    elements.schemaColumnsTbody.appendChild(tr);
  });

  // SQL code block
  elements.sqlCodeBlock.innerText = res.sql || '';

  // Sample data preview
  renderSampleDataPreview(res);
}

function renderSampleDataPreview(res) {
  elements.previewDataThead.innerHTML = '';
  elements.previewDataTbody.innerHTML = '';

  const columns = Array.isArray(res.columns) ? res.columns : [];
  const sampleRows = Array.isArray(res.sampleRows) ? res.sampleRows : [];

  if (sampleRows.length === 0) {
    elements.rowCountBadge.innerText = '0 Rows Materialized';
    elements.previewDataTbody.innerHTML = '<tr><td colspan="10" style="text-align:center;color:var(--text-dim);">No matching sample rows found for join criteria</td></tr>';
    return;
  }

  elements.rowCountBadge.innerText = `${sampleRows.length} Rows Materialized`;

  // Header row
  const headerTr = document.createElement('tr');
  columns.forEach(c => {
    const th = document.createElement('th');
    th.innerText = c.name;
    headerTr.appendChild(th);
  });
  elements.previewDataThead.appendChild(headerTr);

  // Data rows
  sampleRows.forEach(row => {
    const tr = document.createElement('tr');
    columns.forEach(c => {
      const td = document.createElement('td');
      const val = row[c.name];
      if (val === null || val === undefined) {
        td.innerHTML = '<span style="color:var(--text-dim);font-style:italic;">NULL</span>';
      } else {
        td.innerText = val;
      }
      tr.appendChild(td);
    });
    elements.previewDataTbody.appendChild(tr);
  });
}

function renderMaterializedViewError(errorMsg) {
  state.lastResult = null;
  elements.viewEmptyState.style.display = 'none';
  elements.viewResultContent.style.display = 'none';
  if (elements.viewErrorState) {
    elements.viewErrorState.style.display = 'block';
    if (elements.viewErrorMessage) {
      elements.viewErrorMessage.innerText = errorMsg;
    }
  }
}

function resetMaterializedViewSection() {
  state.lastResult = null;
  elements.viewEmptyState.style.display = 'block';
  elements.viewResultContent.style.display = 'none';
  if (elements.viewErrorState) {
    elements.viewErrorState.style.display = 'none';
  }

  // Fully reset all rendered fields
  elements.statCycles.innerText = '0';
  elements.statTime.innerText = '0.00 ms';
  elements.statWmes.innerText = '0';
  elements.statJoins.innerText = '0';
  elements.statRoot.innerText = '-';

  elements.joinPathContainer.innerHTML = '';
  elements.colCountBadge.innerText = '0 Columns';
  elements.schemaColumnsTbody.innerHTML = '';
  elements.sqlCodeBlock.innerText = '';
  elements.rowCountBadge.innerText = '0 Rows Materialized';
  elements.previewDataThead.innerHTML = '';
  elements.previewDataTbody.innerHTML = '';
}

// Copy SQL button
elements.btnCopySql.onclick = () => {
  if (state.lastResult && state.lastResult.sql) {
    navigator.clipboard.writeText(state.lastResult.sql);
    showToast("Copied SQL query to clipboard!");
  }
};

// Schema Switcher
elements.selectSchema.addEventListener('change', (e) => {
  const newSchemaId = e.target.value;
  elements.loadingOverlay.classList.remove('hidden');
  elements.loadingOverlay.querySelector('.loading-text').innerText = `Switching to ${newSchemaId === 'chinook' ? 'Chinook' : 'Northwind'}...`;

  setTimeout(() => {
    window.schemaSwitch(newSchemaId);
    loadSchemaDefinition();
    elements.loadingOverlay.classList.add('hidden');
    showToast(`Loaded ${newSchemaId === 'chinook' ? 'Chinook' : 'Northwind'} schema`);
  }, 100);
});

// Rule Inspector Drawer
elements.btnToggleInspector.onclick = () => {
  const rules = window.schemaGetRuleSource();
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
