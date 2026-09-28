package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: boda <command>")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  check    Run health check")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		if err := runCheck(); err != nil {
			fmt.Fprintln(os.Stderr, "check failed:", err)
			os.Exit(1)
		}
		fmt.Println("ok")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// runCheck performs a minimal health check for the boda service.
// In a full deployment it validates configuration, data directory
// accessibility, and database connectivity.
func runCheck() error {
	// Scaffold: verify basic filesystem accessibility.
	if _, err := os.Getwd(); err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}
	return nil
}
