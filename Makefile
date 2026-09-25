.PHONY: all build-wasm copy-wasm-exec test serve clean

# Go parameters
GOROOT ?= $(shell go env GOROOT)
WASM_EXEC ?= $(GOROOT)/misc/wasm/wasm_exec.js
WASM_OUT ?= apps/life/web/main.wasm

all: test build-wasm

# Compile WebAssembly binary using Go toolchain
build-wasm: copy-wasm-exec
	@echo "==> Compiling Game of Life WebAssembly binary (GOOS=js GOARCH=wasm)..."
	GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./apps/life/wasm
	@echo "==> Build complete: $(WASM_OUT)"

# Copy official Go WebAssembly JS support glue
copy-wasm-exec:
	@if [ -f "$(WASM_EXEC)" ]; then \
		echo "==> Syncing wasm_exec.js from $(WASM_EXEC)..."; \
		cp "$(WASM_EXEC)" apps/life/web/wasm_exec.js; \
	fi

# Run test suite
test:
	@echo "==> Running automated test suite..."
	go test -v ./...

# Build wasm and launch local web server
serve: build-wasm
	@echo "==> Starting local HTTP server..."
	go run ./apps/life/server -port 8080

# Clean generated wasm artifacts
clean:
	@rm -f $(WASM_OUT)
	@echo "==> Cleaned $(WASM_OUT)"
