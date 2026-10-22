package main

import (
	"log"
	"net/http"
	"os"

	"codeflow/internal/workspace"
)

func main() {
	addr := env("CODEFLOW_ADDR", ":8787")
	data := env("CODEFLOW_DATA", "data/project.json")
	store, err := workspace.NewStore(data)
	if err != nil {
		log.Fatal(err)
	}
	server := workspace.NewServer(store, "dist")
	log.Printf("CodeFlow listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
