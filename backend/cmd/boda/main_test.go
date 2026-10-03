package main

import (
	"os"
	"testing"
)

func TestRunCheck(t *testing.T) {
	if err := runCheck(); err != nil {
		t.Fatalf("runCheck() = %v; want nil", err)
	}
}

func TestRunCheck_WritableTemp(t *testing.T) {
	// Verify the environment allows basic filesystem operations.
	f, err := os.CreateTemp("", "boda-check-*")
	if err != nil {
		t.Fatalf("cannot create temp file: %v", err)
	}
	f.Close()
	os.Remove(f.Name())
}
