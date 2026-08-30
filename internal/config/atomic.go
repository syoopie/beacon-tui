package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes via a temp file in the same directory plus rename, so a
// concurrent reader never sees a torn file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%s: creating parent directory: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("%s: creating temp file: %w", path, err)
	}
	tmpName := tmp.Name()

	// Chmod before the rename, so a 0600 file is never briefly world-readable.
	err = tmp.Chmod(perm)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("%s: writing %s: %w", path, tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("%s: renaming temp file into place: %w", path, err)
	}
	return nil
}
