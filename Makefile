.PHONY: all build-wasm build-wasm-life build-wasm-schema build-wasm-cassandra copy-wasm-exec test serve serve-life serve-schema serve-cassandra clean

# Go parameters
GOROOT ?= $(shell go env GOROOT)
WASM_EXEC ?= $(GOROOT)/misc/wasm/wasm_exec.js
WASM_LIFE_OUT ?= apps/life/web/main.wasm
WASM_SCHEMA_OUT ?= apps/schema/web/main.wasm
WASM_CASSANDRA_OUT ?= apps/cassandra/web/main.wasm

all: test build-wasm

# Compile all WebAssembly binaries
build-wasm: build-wasm-life build-wasm-schema build-wasm-cassandra

# Compile Game of Life WebAssembly binary
build-wasm-life: copy-wasm-exec
	@echo "==> Compiling Game of Life WebAssembly binary (GOOS=js GOARCH=wasm)..."
	GOOS=js GOARCH=wasm go build -o $(WASM_LIFE_OUT) ./apps/life/wasm
	@echo "==> Build complete: $(WASM_LIFE_OUT)"

# Compile Schema & Materialized View Synthesizer WebAssembly binary
build-wasm-schema: copy-wasm-exec
	@echo "==> Compiling Schema View Synthesizer WebAssembly binary (GOOS=js GOARCH=wasm)..."
	GOOS=js GOARCH=wasm go build -o $(WASM_SCHEMA_OUT) ./apps/schema/wasm
	@echo "==> Build complete: $(WASM_SCHEMA_OUT)"

# Compile Cassandra Protocol Dual-Ring Simulator WebAssembly binary
build-wasm-cassandra: copy-wasm-exec
	@echo "==> Compiling Cassandra Protocol WebAssembly binary (GOOS=js GOARCH=wasm)..."
	GOOS=js GOARCH=wasm go build -o $(WASM_CASSANDRA_OUT) ./apps/cassandra/wasm
	@echo "==> Build complete: $(WASM_CASSANDRA_OUT)"

# Copy official Go WebAssembly JS support glue to web directories
copy-wasm-exec:
	@if [ -f "$(WASM_EXEC)" ]; then \
		echo "==> Syncing wasm_exec.js from $(WASM_EXEC)..."; \
		cp "$(WASM_EXEC)" apps/life/web/wasm_exec.js; \
		cp "$(WASM_EXEC)" apps/schema/web/wasm_exec.js; \
		cp "$(WASM_EXEC)" apps/cassandra/web/wasm_exec.js; \
	fi

# Run test suite across all apps
test:
	@echo "==> Running automated test suite..."
	go test -v ./...

# Launch local HTTP server for Game of Life (port 8080)
serve-life: build-wasm-life
	@echo "==> Starting local HTTP server for Game of Life (port 8080)..."
	go run ./apps/life/server -port 8080

# Launch local HTTP server for Schema View Synthesizer (port 8081)
serve-schema: build-wasm-schema
	@echo "==> Starting local HTTP server for Schema View Synthesizer (port 8081)..."
	go run ./apps/schema/server -port 8081

# Launch local HTTP server for Cassandra Protocol Simulator (port 8082)
serve-cassandra: build-wasm-cassandra
	@echo "==> Starting local HTTP server for Cassandra Protocol Simulator (port 8082)..."
	go run ./apps/cassandra/server -port 8082

# Default serve runs schema app
serve: serve-schema

# Clean generated wasm artifacts
clean:
	@rm -f $(WASM_LIFE_OUT) $(WASM_SCHEMA_OUT) $(WASM_CASSANDRA_OUT)
	@echo "==> Cleaned WebAssembly artifacts"
