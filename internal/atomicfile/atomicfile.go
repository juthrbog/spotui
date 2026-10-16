// Package atomicfile writes files so that a crash never leaves a half-written
// file.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Write writes data to a temporary file next to path, then renames it to
// path. On Unix, only the user can read the file.
func Write(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary file for %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing %s: %w", path, err)
	}
	// Windows cannot rename a file that is still open.
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := replace(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// replace moves src onto dst. On Windows, a rename can fail when dst exists,
// so dst is removed first and the rename is tried again. Other systems keep
// the atomic rename and never remove dst.
func replace(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(src, dst)
}
