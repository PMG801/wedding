package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeRejectsMissingRootWithoutCreatingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, err := Initialize(root); err == nil {
		t.Fatal("Initialize() error = nil; want missing root error")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("data root was created or unexpected stat error: %v", err)
	}
}

func TestInitializeCreatesManagedDirectoriesRestrictively(t *testing.T) {
	root := t.TempDir()
	paths, err := Initialize(root)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	for _, name := range managedDirectories {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("managed directory %q: %v", name, err)
		}
		if !info.IsDir() {
			t.Errorf("managed path %q is not a directory", name)
		}
		if got := info.Mode().Perm(); got != 0700 {
			t.Errorf("managed directory %q permissions = %04o; want 0700", name, got)
		}
	}
	if paths.Tmp != filepath.Join(root, "tmp") || paths.Originals != filepath.Join(root, "originals") {
		t.Errorf("unexpected storage paths: %+v", paths)
	}
}

func TestInitializeRejectsNonDirectoryRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err == nil {
		t.Fatal("Initialize() error = nil; want non-directory root error")
	}
}

func TestInitializeRejectsChildThatIsNotDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tmp"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err == nil {
		t.Fatal("Initialize() error = nil; want managed child error")
	}
}

func TestInitializeRejectsManagedDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "tmp")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Initialize(root); err == nil {
		t.Fatal("Initialize() error = nil; want managed symlink error")
	}
}

func TestInitializeRestrictsExistingManagedDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tmp"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Fatalf("existing tmp permissions = %04o; want 0700", got)
	}
}

func TestInitializeRejectsUnwritableRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	if _, err := Initialize(root); err == nil {
		t.Fatal("Initialize() error = nil; want unwritable root error")
	}
}

func TestSameFilesystemIDs(t *testing.T) {
	if err := sameFilesystemIDs(7, 7); err != nil {
		t.Fatalf("sameFilesystemIDs() error = %v; want nil", err)
	}
	if err := sameFilesystemIDs(7, 8); err == nil {
		t.Fatal("sameFilesystemIDs() error = nil; want filesystem mismatch")
	}
}
