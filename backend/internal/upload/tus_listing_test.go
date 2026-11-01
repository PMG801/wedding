package upload

import "os"

// readDirWithoutTus lists a directory for the cleanup assertions. tmp/tus is a
// managed directory of resumable uploads (ADR-0014), covered by its own tests,
// so it is not a leftover of a rejected photo or derivative.
func readDirWithoutTus(directory string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(directory)
	kept := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != "tus" {
			kept = append(kept, entry)
		}
	}
	return kept, err
}
