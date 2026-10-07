package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	port := flag.Int("port", 8082, "HTTP server port")
	webDir := flag.String("dir", "apps/cassandra/web", "Directory containing static web files")
	flag.Parse()

	// Ensure .wasm MIME type is explicitly mapped for WebAssembly streaming compilation
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	absDir, err := filepath.Abs(*webDir)
	if err != nil {
		log.Fatalf("Invalid web directory path: %v", err)
	}

	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		log.Fatalf("Web directory does not exist: %s", absDir)
	}

	fileServer := http.FileServer(http.Dir(absDir))

	// Handler with logging and cache-control headers
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent aggressive browser caching during development
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Println("==================================================================")
	fmt.Println(" Cassandra Protocol Dual-Ring Simulation — Web Dev Server")
	fmt.Println("==================================================================")
	fmt.Printf(" Serving directory: %s\n", absDir)
	fmt.Printf(" URL:               http://localhost:%d\n", *port)
	fmt.Println(" Press Ctrl+C to terminate the server")
	fmt.Println("==================================================================")

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server terminated: %v", err)
	}
}
