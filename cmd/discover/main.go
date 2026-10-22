package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"codeflow/internal/discovery"
)

func main() {
	root := flag.String("root", ".", "repository directory to inspect")
	maxFiles := flag.Int("max-files", 25000, "maximum files to inventory")
	compact := flag.Bool("compact", false, "print compact JSON")
	flag.Parse()
	report, err := discovery.Scan(*root, discovery.Options{MaxFiles: *maxFiles})
	if err != nil {
		log.Fatal(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	if !*compact {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
