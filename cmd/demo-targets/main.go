// Command demo-targets serves deterministic local endpoints for the documented status-page demo.
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /up", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("ready"))
	})
	mux.HandleFunc("GET /slow", func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(120 * time.Millisecond)
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("complete"))
	})
	mux.HandleFunc("GET /down", func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "simulated outage", http.StatusServiceUnavailable)
	})

	server := &http.Server{
		Addr:              "127.0.0.1:18081",
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	fmt.Println("demo targets listening on http://127.0.0.1:18081")
	log.Fatal(server.ListenAndServe())
}
