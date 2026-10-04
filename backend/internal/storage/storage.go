// Package storage validates and prepares the application's managed data directories.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var managedDirectories = []string{"data", "tmp", "originals", "derived", "hidden", "quarantine"}

// Paths identifies the managed directories below the configured data root.
type Paths struct {
	Root       string
	Data       string
	Tmp        string
	Originals  string
	Derived    string
	Hidden     string
	Quarantine string
}

// Initialize validates an existing writable root, creates its managed child
// directories with private permissions, and verifies the rename boundary.
// The configured root itself is never created.
func Initialize(root string) (Paths, error) {
	if root == "" {
		return Paths{}, fmt.Errorf("BODA_DATA_DIR is required")
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return Paths{}, fmt.Errorf("data root %q is unavailable: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return Paths{}, fmt.Errorf("data root %q is not a directory", root)
	}
	if rootInfo.Mode().Perm()&0222 == 0 {
		return Paths{}, fmt.Errorf("data root %q is not writable", root)
	}
	probe, err := os.CreateTemp(root, ".boda-write-check-*")
	if err != nil {
		return Paths{}, fmt.Errorf("data root %q is not writable: %w", root, err)
	}
	probeName := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(probeName)
	if closeErr != nil {
		return Paths{}, fmt.Errorf("cannot close data root write check: %w", closeErr)
	}
	if removeErr != nil {
		return Paths{}, fmt.Errorf("cannot remove data root write check: %w", removeErr)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("cannot resolve data root %q: %w", root, err)
	}
	paths := Paths{Root: absRoot}
	for _, name := range managedDirectories {
		path := filepath.Join(absRoot, name)
		if err := ensureManagedDirectory(path); err != nil {
			return Paths{}, err
		}
		switch name {
		case "data":
			paths.Data = path
		case "tmp":
			paths.Tmp = path
		case "originals":
			paths.Originals = path
		case "derived":
			paths.Derived = path
		case "hidden":
			paths.Hidden = path
		case "quarantine":
			paths.Quarantine = path
		}
	}

	tmpDevice, err := filesystemDevice(paths.Tmp)
	if err != nil {
		return Paths{}, fmt.Errorf("cannot identify filesystem for tmp directory: %w", err)
	}
	originalsDevice, err := filesystemDevice(paths.Originals)
	if err != nil {
		return Paths{}, fmt.Errorf("cannot identify filesystem for originals directory: %w", err)
	}
	if err := sameFilesystemIDs(tmpDevice, originalsDevice); err != nil {
		return Paths{}, err
	}
	return paths, nil
}

func ensureManagedDirectory(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0700); err != nil {
			return fmt.Errorf("cannot create managed data directory %q: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect managed data directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("managed data path %q must be a directory, not a symlink", path)
	}
	if err := os.Chmod(path, 0700); err != nil {
		return fmt.Errorf("cannot restrict permissions on managed data directory %q: %w", path, err)
	}
	return nil
}

func filesystemDevice(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("filesystem device information is unavailable")
	}
	return uint64(stat.Dev), nil
}

func sameFilesystemIDs(tmp, originals uint64) error {
	if tmp != originals {
		return fmt.Errorf("tmp and originals directories must be on the same filesystem")
	}
	return nil
}
