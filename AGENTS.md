# OPS5 Applications (`go-ops5-apps`) - Agent Context & Guidelines

> **Project Identity**: Production-grade showcase applications powered by the pure Go [**`ops5` Rete Pattern Matching Engine**](https://github.com/graemenewlands/ops5) compiled to WebAssembly.
> **Repository**: [`graemenewlands/go-ops5-apps`](https://github.com/graemenewlands/go-ops5-apps)
> **Published Live**: Hosted on Graeme Newlands' personal site at [**graemenewlands.com**](https://graemenewlands.com/) (`/life/` and `/schema/`).

---

## 1. Directory Layout & Architecture

```
go-ops5-apps/
├── Makefile                   # Build automation (test, build-wasm, serve-life, serve-schema)
├── README.md                  # Showcase documentation and architecture diagrams
├── go.mod                     # Go module definition (requires github.com/graemenewlands/ops5)
├── go.sum                     # Checksums
└── apps/
    ├── life/                  # Conway's Game of Life in OPS5
    │   ├── rules/life.ops     # 5-stage declarative cellular automaton rules
    │   ├── engine.go          # Go LifeEngine wrapper
    │   ├── life_test.go       # Conway pattern unit tests
    │   ├── wasm/main.go       # WebAssembly syscall/js entrypoint
    │   ├── server/main.go     # Local HTTP development server (port 8080)
    │   └── web/               # HTML5 Canvas interface and wasm artifacts
    └── schema/                # Schema & Materialized View Synthesizer
        ├── rules/schema.ops   # OPS5 join rules and view synthesis pipeline
        ├── data/              # Chinook & Northwind schemas and sample datasets
        ├── engine.go          # Go SchemaEngine managing working memory & joins
        ├── schema_test.go     # Unit tests verifying join inference and SQL synthesis
        ├── wasm/main.go       # WebAssembly syscall/js entrypoint
        ├── server/main.go     # Local HTTP development server (port 8081)
        └── web/               # Interactive E-R Diagram & Materialized View UI
```

---

## 2. Applications

1. **Conway's Game of Life (`apps/life`)**:
   - Computes every generation purely via forward-chaining OPS5 production rules without imperative loops.
   - Lifecycle: `emit` ➔ `tally` ➔ `evaluate` ➔ `clear-cells` & `promote` ➔ `ready` (quiescence).
   - Compiled to `apps/life/web/main.wasm`.

2. **Schema & Materialized View Synthesizer (`apps/schema`)**:
   - Represents tables, columns, foreign keys (1-N / N-1), and Many-to-Many associations as WMEs.
   - Joins and path discovery are modeled directly as production rules matching needed tables and relations.
   - Synthesizes formatted `CREATE MATERIALIZED VIEW` SQL statements with automatic column alias conflict resolution and in-memory sample data joins.
   - Compiled to `apps/schema/web/main.wasm`.

---

## 3. Critical Invariants & Constraints

When modifying or extending this codebase, adhere strictly to these rules:

1. **Local-First Testing Constraint**: **ALWAYS test changes locally before proposing or deploying to production (`https://graemenewlands.com/`).**
   - **NEVER** copy build artifacts to `graemenewlands.github.io`, rebuild production output, or push to `origin` without prior local verification and explicit approval from the user.
2. **Push Constraint**: **NEVER push commits or tags to `origin` without explicit user permission.**
3. **WASM Build Integrity**: When modifying Go engine code, always compile WebAssembly artifacts (`make build-wasm`) and ensure unit tests pass (`make test`).
4. **MIME Mapping**: The local development servers in `apps/*/server/main.go` must explicitly map `.wasm` to `application/wasm` via `mime.AddExtensionType` to ensure streaming WebAssembly compilation succeeds.
5. **No Framework Bloat**: Web frontends use vanilla JavaScript and standard Web APIs (HTML5 Canvas, native SVG `<foreignObject>`) to maintain zero runtime dependencies and fast loading speeds.

---

## 4. Common Commands & Workflows

### Automated Testing
Run the complete automated test suite across all applications:
```bash
make test
```

### Building WebAssembly Artifacts
Rebuild both Wasm binaries and sync `wasm_exec.js` from `$GOROOT`:
```bash
make build-wasm
# Or individually:
make build-wasm-life
make build-wasm-schema
```

### Local Development Servers
To test applications locally in the browser:
```bash
# Schema & Materialized View Synthesizer (port 8081)
make serve-schema
# Open http://localhost:8081

# Conway's Game of Life (port 8080)
make serve-life
# Open http://localhost:8080
```
